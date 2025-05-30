package funcfield

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
)

// // ID identifies a function argument or return value.
type ID struct {
	// ModPath is the module path containing the struct field package.
	//
	// If set to "std", the struct field belongs to the standard Go library.
	ModPath string
	// PkgPath package import path containing the struct field.
	PkgPath string
	// Struct is the name of the struct containing the field.
	Func string
	// Field is the field name.
	Arg string
}

// NewID returns a new ID using pkg for the PkgPath, strct for the Struct, and
// field for the Field.
func NewID(mod, pkg, fn, arg string) ID {
	return ID{ModPath: mod, PkgPath: pkg, Func: fn, Arg: arg}
}

type UniqueOffset struct {
	Value    uint64
	Location Location
	Valid    bool
}

// Offsets holds byte offsets for function arguments and return values.
type Offsets struct {
	Mu     sync.RWMutex
	Values map[VerKey]OffsetVersion
	Ua     UniqueOffset
}

func (o *Offsets) GetLatest() (OffsetKey, VerKey) {
	o.Mu.RLock()
	defer o.Mu.RUnlock()

	latestVersion := VerKey{}
	val := OffsetKey{}
	for verKey, ov := range o.Values {
		// TODO(ddelnano): This needs to handle differences in the Location as well.
		// for now this doesn't matter since for the ABIInternal (more recent Go ABI)
		// would have to pass many arguments to spill over to the stack.
		if verKey.GreaterThan(latestVersion) && ov.Offset.Valid {
			latestVersion = verKey
			val = ov.Offset
		}
	}

	return val, latestVersion
}

func (o *Offsets) Index() map[OffsetKey][]*semver.Version {
	o.Mu.RLock()
	defer o.Mu.RUnlock()

	out := make(map[OffsetKey][]*semver.Version)
	for _, ov := range o.Values {
		vers, ok := out[ov.Offset]
		if ok {
			i := sort.Search(len(vers), func(i int) bool {
				return vers[i].GreaterThanEqual(ov.Version)
			})
			vers = append(vers, nil)
			copy(vers[i+1:], vers[i:])
			vers[i] = ov.Version
		} else {
			vers = append(vers, ov.Version)
		}
		out[ov.Offset] = vers
	}
	return out
}

func (v VerKey) GreaterThan(other VerKey) bool {
	return v.Version.GreaterThan(&other.Version)
}

// NewOffsets initializes an empty Offsets structure.
func NewOffsets() *Offsets {
	return &Offsets{Values: make(map[VerKey]OffsetVersion)}
}

// Get retrieves an offset and location for a given version.
func (o *Offsets) Get(ver *semver.Version) (OffsetKey, bool) {
	if o == nil {
		return OffsetKey{}, false
	}
	o.Mu.RLock()
	v, ok := o.Values[NewVerKey(ver)]
	o.Mu.RUnlock()

	// TODO(ddelnano): The Location should be checked here. For now this doesn't
	// matter since for the ABIInternal (more recent Go ABI) would have to pass
	// many arguments to spill over to the stack.
	if strings.HasPrefix(ver.String(), "0.0.0") && !ok && o.Ua.Valid {
		return OffsetKey{Offset: o.Ua.Value, Valid: true, Location: o.Ua.Location}, true
	}

	return v.Offset, ok
}

// // Put stores an offset and location type for a given version.
func (o *Offsets) Put(ver *semver.Version, offset OffsetKey) {
	ov := OffsetVersion{Offset: offset, Version: ver}
	o.Mu.Lock()
	defer o.Mu.Unlock()

	if o.Values == nil {
		o.Values = map[VerKey]OffsetVersion{NewVerKey(ver): ov}
		o.Ua.Valid = ov.Offset.Valid
		o.Ua.Value = ov.Offset.Offset
		o.Ua.Location = ov.Offset.Location
		return
	}

	o.Values[NewVerKey(ver)] = ov
	if o.Ua.Valid && (o.Ua.Value != ov.Offset.Offset || o.Ua.Location != ov.Offset.Location) {
		o.Ua.Valid = false
	}
}

// // Struct for version comparison.
type VerKey struct {
	semver.Version
}

func NewVerKey(v *semver.Version) VerKey {
	stripped := semver.New(v.Major(), v.Minor(), v.Patch(), v.Prerelease(), v.Metadata())
	return VerKey{Version: *stripped}
}

type Location int

const (
	Unknown   Location = iota
	Stack     Location = iota
	Registers Location = iota
)

func (l *Location) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case `"kStack"`:
		fallthrough
	case `"stack"`:
		*l = Stack
	case `"kRegister"`:
		fallthrough
	case `"registers"`:
		*l = Registers
	case `"kUnknown"`:
		fallthrough
	case `"unknown"`:
		*l = Unknown
	default:
		return errors.New(fmt.Sprintf("invalid location %s", string(data)))
	}
	return nil
}

func (l *Location) MarshalJSON() ([]byte, error) {
	if *l == Unknown {
		return []byte(`"unknown"`), nil
	} else if *l == Stack {
		return []byte(`"stack"`), nil
	} else if *l == Registers {
		return []byte(`"registers"`), nil
	}
	return nil, errors.New(fmt.Sprintf("invalid location %d", *l))
}

type OffsetKey struct {
	Offset   uint64
	Location Location
	Valid    bool
}

type OffsetVersion struct {
	Offset  OffsetKey
	Version *semver.Version
}
