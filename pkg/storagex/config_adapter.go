package storagex

type ConfigUnmarshaler interface {
	Unmarshal(out any) error
}
