package ssdp

// Optional is a field that not every UDA version carries.
type Optional[T any] struct {
	value   T
	present bool
}

func NewOptional[T any](value T) Optional[T] {
	return Optional[T]{
		value:   value,
		present: true,
	}
}

func (o Optional[T]) Get() (T, bool) {
	return o.value, o.present
}

func (o Optional[T]) IsPresent() bool {
	return o.present
}
