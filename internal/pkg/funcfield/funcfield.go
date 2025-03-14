package funcfield

// LocationType defines where an argument is stored.
// type LocationType string

// const (
// 	LocationStack    LocationType = "stack"
// 	LocationRegister LocationType = "register"
// )

// // OffsetKey represents a function argument or return value offset.
// type OffsetKey struct {
// 	Offset uint64 `json:"offset"`
// 	Valid  bool   `json:"valid"`
// 	// Loc    LocationType `json:"location"` // "stack" or "register"
// 	Loc string
// }

// // ID identifies a function argument or return value.
type ID struct {
	Name    string // Module path
	Args    []string
	Retvals []int
}

// type uniqueOffset struct {
// 	value uint64
// 	valid bool
// 	loc   string // Added: Track "stack" or "register"
// }

// // Offsets holds byte offsets for function arguments and return values.
// type Offsets struct {
// 	mu     sync.RWMutex
// 	values map[verKey]offsetVersion
// 	uo     uniqueOffset
// }

// // NewOffsets initializes an empty Offsets structure.
// func NewOffsets() *Offsets {
// 	return &Offsets{values: make(map[verKey]offsetVersion)}
// }

// // Get retrieves an offset and location for a given version.
// func (o *Offsets) Get(ver *semver.Version) (OffsetKey, bool) {
// 	if o == nil {
// 		return OffsetKey{}, false
// 	}
// 	o.mu.RLock()
// 	v, ok := o.values[newVerKey(ver)]
// 	o.mu.RUnlock()

// 	if strings.HasPrefix(ver.String(), "0.0.0") && !ok && o.uo.valid {
// 		return OffsetKey{Offset: o.uo.value, Valid: true, Loc: o.uo.loc}, true
// 	}

// 	return v.offset, ok
// }

// // Put stores an offset and location type for a given version.
// func (o *Offsets) Put(ver *semver.Version, offset OffsetKey) {
// 	ov := offsetVersion{offset: offset, version: ver}
// 	o.mu.Lock()
// 	defer o.mu.Unlock()

// 	if o.values == nil {
// 		o.values = map[verKey]offsetVersion{newVerKey(ver): ov}
// 		o.uo.valid = ov.offset.Valid
// 		o.uo.value = ov.offset.Offset
// 		o.uo.loc = ov.offset.Loc
// 		return
// 	}

// 	o.values[newVerKey(ver)] = ov
// 	if o.uo.valid && (o.uo.value != ov.offset.Offset || o.uo.loc != ov.offset.Loc) {
// 		o.uo.valid = false
// 	}
// }

// // Struct for version comparison.
// type verKey struct {
// 	semver.Version
// }

// func newVerKey(v *semver.Version) verKey {
// 	stripped := semver.New(v.Major(), v.Minor(), v.Patch(), v.Prerelease(), v.Metadata())
// 	return verKey{Version: *stripped}
// }

// type offsetVersion struct {
// 	offset  OffsetKey
// 	version *semver.Version
// }
