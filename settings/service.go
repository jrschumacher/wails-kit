package settings

import "sync"

type Service struct {
	schema   Schema
	store    *Store
	onChange []func(values map[string]any)
	mu       sync.Mutex
}

type ServiceOption func(*Service)

func WithAppName(name string) ServiceOption {
	return func(s *Service) {
		s.store = NewStore(name)
	}
}

func WithStorePath(path string) ServiceOption {
	return func(s *Service) {
		if s.store == nil {
			s.store = NewStore("app", WithPath(path))
		} else {
			s.store.path = path
		}
	}
}

func WithGroup(g Group) ServiceOption {
	return func(s *Service) {
		s.schema.Groups = append(s.schema.Groups, g)
	}
}

func WithOnChange(fn func(values map[string]any)) ServiceOption {
	return func(s *Service) {
		s.onChange = append(s.onChange, fn)
	}
}

func NewService(opts ...ServiceOption) *Service {
	s := &Service{}
	for _, opt := range opts {
		opt(s)
	}
	if s.store == nil {
		s.store = NewStore("app")
	}

	// Register defaults from schema fields
	defaults := make(map[string]any)
	for _, group := range s.schema.Groups {
		for _, field := range group.Fields {
			if field.Default != nil {
				defaults[field.Key] = field.Default
			}
		}
	}
	s.store.SetDefaults(defaults)

	return s
}

func (s *Service) GetSchema() Schema {
	return s.schema
}

func (s *Service) GetValues() (map[string]any, error) {
	values, err := s.store.Load()
	if err != nil {
		return values, err
	}

	// Run compute functions
	for _, group := range s.schema.Groups {
		for key, fn := range group.ComputeFuncs {
			values[key] = fn(values)
		}
	}

	return values, nil
}

func (s *Service) SetValues(values map[string]any) ([]ValidationError, error) {
	if errs := Validate(s.schema, values); errs != nil {
		return errs, nil
	}

	// Strip computed fields before persisting
	toSave := make(map[string]any)
	computed := s.computedKeys()
	for k, v := range values {
		if !computed[k] {
			toSave[k] = v
		}
	}

	if err := s.store.Save(toSave); err != nil {
		return nil, err
	}

	// Notify listeners
	for _, fn := range s.onChange {
		fn(values)
	}

	return nil, nil
}

func (s *Service) computedKeys() map[string]bool {
	keys := make(map[string]bool)
	for _, group := range s.schema.Groups {
		for _, field := range group.Fields {
			if field.Type == FieldComputed {
				keys[field.Key] = true
			}
		}
	}
	return keys
}
