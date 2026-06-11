package settings

// Validate checks value for key. Full per-key rules land with the
// validation task; the settings store calls it before every write so
// invalid values are never stored.
func Validate(key, value string) error {
	return nil
}
