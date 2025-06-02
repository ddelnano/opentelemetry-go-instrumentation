// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package process

import (
	"debug/dwarf"
	"errors"
	"fmt"
	"io"
	"log"
	"slices"
	"strings"

	"go.opentelemetry.io/auto/internal/pkg/funcfield"
	"go.opentelemetry.io/auto/internal/pkg/structfield"
)

// ErrDWARFEntry is returned if an entry is not found within DWARF data.
var ErrDWARFEntry = errors.New("DWARF entry not found")

type ABI int

const (
	AbiUnknown ABI = iota
	AbiGoStack ABI = iota
	AbiGoReg   ABI = iota
)

type ArgTracker interface {
	PopLocation(typeClass TypeClass, typeSize uint64, alignmentSize uint64, numVars int, retArg bool) (FuncFieldArg, error)
}

// NewArgTracker returns an ArgTracker based on the ABI.
func NewArgTracker(abi ABI, addrSize int) ArgTracker {
	if abi == AbiGoReg {
		return &GoRegABIArgTracker{
			CurrentStackOffset: 0,
			AddrSize:           addrSize,
			IntArgRegs:         []Registers{kRAX, kRBX, kRCX, kRDX, kRSI, kR8, kR9, kR10, kR11},
			IntRetValRegs:      []Registers{kRAX, kRBX, kRCX, kRDX, kRSI, kR8, kR9, kR10, kR11},
			FloatArgRegs:       []Registers{kXMM0, kXMM1, kXMM2, kXMM3, kXMM4, kXMM5, kXMM6, kXMM7, kXMM8, kXMM9, kXMM10, kXMM11, kXMM12, kXMM13, kXMM14},
			FloatRetValRegs:    []Registers{kXMM0, kXMM1, kXMM2, kXMM3, kXMM4, kXMM5, kXMM6, kXMM7, kXMM8, kXMM9, kXMM10, kXMM11, kXMM12, kXMM13, kXMM14},
		}
	} else if abi == AbiGoStack {
		return &GoStackABIArgTracker{
			CurrentStackOffset: 0,
			AddrSize:           addrSize,
		}
	} else {
		return nil
	}
}

type Registers int

const (
	kRAX = 0
	kRBX = 1
	kRCX = 2
	kRDX = 3
	kRDI = 4
	kRSI = 5
	kR8  = 6
	kR9  = 7
	kR10 = 8
	kR11 = 9

	kXMM0  = 100
	kXMM1  = 101
	kXMM2  = 102
	kXMM3  = 103
	kXMM4  = 104
	kXMM5  = 105
	kXMM6  = 106
	kXMM7  = 107
	kXMM8  = 108
	kXMM9  = 109
	kXMM10 = 110
	kXMM11 = 111
	kXMM12 = 112
	kXMM13 = 113
	kXMM14 = 114
)

type GoRegABIArgTracker struct {
	AddrSize                 int
	CurrentStackOffset       uint64
	CurrentIntRegOffset      uint64
	CurrentFloatRegOffset    uint64
	CurrentIntRetValOffset   uint64
	CurrentFloatRetValOffset uint64

	RegisterSize    uint64
	IntArgRegs      []Registers
	FloatArgRegs    []Registers
	IntRetValRegs   []Registers
	FloatRetValRegs []Registers
}

func (g *GoRegABIArgTracker) PopLocation(typeClass TypeClass, typeSize uint64, alignmentSize uint64, numVars int, retArg bool) (FuncFieldArg, error) {
	var regOffset *uint64
	var registers *[]Registers
	if typeClass == TypeClassInt {
		if retArg {
			registers = &g.IntRetValRegs
			regOffset = &g.CurrentIntRetValOffset
		} else {
			registers = &g.IntArgRegs
			regOffset = &g.CurrentIntRegOffset
		}
	} else if typeClass == TypeClassFloat {
		if retArg {
			registers = &g.FloatRetValRegs
			regOffset = &g.CurrentFloatRetValOffset
		} else {
			registers = &g.FloatArgRegs
			regOffset = &g.CurrentFloatRegOffset
		}
	}
	funcFieldArg := FuncFieldArg{
		RetArg: retArg,
	}
	if numVars <= len(*registers) {
		funcFieldArg.Location = funcfield.Registers
		funcFieldArg.Offset = *regOffset

		// TODO(ddelnano): Consider if its worth tracking which registers belong
		// to a funcFieldArg. This is done in Pixie's implementation, but is not required
		// for this uprobe optimization feature set.

		// Pop a register off for each variable in the type.
		for i := 0; i < numVars; i++ {
			*registers = (*registers)[1:]
		}
		*regOffset += uint64(numVars * g.AddrSize)
		return funcFieldArg, nil
	} else {
		funcFieldArg.Location = funcfield.Stack
		return funcFieldArg, errors.New("Stack arguments not implemented for GoRegABIArgTracker")
	}
}

type GoStackABIArgTracker struct {
	AddrSize           int
	CurrentStackOffset uint64
}

func IntRoundDivide(x uint64, y uint64) uint64 {
	return (x + (y - 1)) / y
}

func SnapUpToMultiple(x, y uint64) uint64 {
	return IntRoundDivide(x, y) * y
}

// TODO(ddelnano): This is yet to be tested. As upstream has never supported these binaries.
func (g *GoStackABIArgTracker) PopLocation(typeClass TypeClass, typeSize uint64, alignmentSize uint64, numVars int, retArg bool) (FuncFieldArg, error) {

	g.CurrentStackOffset = SnapUpToMultiple(g.CurrentStackOffset, alignmentSize)
	offset := g.CurrentStackOffset
	g.CurrentStackOffset += typeSize
	return FuncFieldArg{
		RetArg:   retArg,
		Offset:   offset,
		Location: funcfield.Stack,
	}, nil
}

// typeInfo stores pre-computed properties for a DWARF type node.
type typeInfo struct {
	size  uint64
	align uint64
	class TypeClass
	nVars int
}

type DWARF interface {
	GoFuncFieldArgs(fn string) (map[string]FuncFieldArg, error)
	GoStructField(id structfield.ID) (int64, error)
}

func NewDWARF(d *dwarf.Data) DWARF {
	return dwarfReader{
		Data:      d,
		Reader:    d.Reader(),
		typeCache: make(map[dwarf.Offset]typeInfo),
	}
}

// dwarfReader provides convenience in accessing DWARF debugging data.
type dwarfReader struct {
	Data       *dwarf.Data
	Reader     *dwarf.Reader
	argTracker ArgTracker
	typeCache  map[dwarf.Offset]typeInfo
}

type FuncFieldArg struct {
	Offset   uint64             `json:"offset"`
	Location funcfield.Location `json:"location"`
	RetArg   bool               `json:"ret_arg"` // true if this is a return argument
}

func (d dwarfReader) DetectSourceABI() (ABI, error) {
	cus, err := d.EntriesWithTag(dwarf.TagCompileUnit)
	if err != nil || len(cus) == 0 {
		return AbiUnknown, fmt.Errorf("No compile units found")
	}
	abi := AbiUnknown
	for _, cu := range cus {
		cuProducer, ok := d.Field(cu, dwarf.AttrProducer)
		if !ok {
			continue
		}
		producer, ok := cuProducer.Val.(string)
		if strings.Contains(producer, "regabi") {
			abi = AbiGoReg
		} else {
			abi = AbiGoStack
		}
	}
	if abi == AbiUnknown {
		return abi, fmt.Errorf("didn't find a AT_producer in any compile unit")
	}
	return abi, nil
}

func (d dwarfReader) IsRetArg(die *dwarf.Entry) bool {
	if f, ok := d.Field(die, dwarf.AttrVarParam); ok {
		varParam := f.Val.(bool)
		return varParam
	}
	return false
}

func (d dwarfReader) GetTypeDIE(entry *dwarf.Entry) (*dwarf.Entry, error) {
	var field dwarf.Field
	var found bool
	var err error
	for {
		field, found = d.Field(entry, dwarf.AttrType)
		if !found {
			return nil, fmt.Errorf("failed to get type attribute: %w", ErrDWARFEntry)
		}
		typeOffset, ok := field.Val.(dwarf.Offset)
		if !ok {
			return nil, fmt.Errorf("type attribute is not a valid dwarf.Offset: %w", ErrDWARFEntry)
		}
		d.Reader.Seek(typeOffset)
		entry, err = d.Reader.Next()
		if errors.Is(err, io.EOF) || entry == nil || entry.Tag == 0 {
			return nil, ErrDWARFEntry
		}
		if entry.Tag != dwarf.TagTypedef {
			return entry, nil
		}
	}
	return nil, ErrDWARFEntry
}

type TypeClass int

const (
	TypeClassNone  TypeClass = iota
	TypeClassInt   TypeClass = iota
	TypeClassFloat TypeClass = iota
	TypeClassMixed TypeClass = iota
)

func combineTypeClasses(a, b TypeClass) TypeClass {
	if a == TypeClassMixed || b == TypeClassMixed {
		return TypeClassMixed
	}
	if b == TypeClassNone {
		return a
	}
	if a == TypeClassNone {
		return b
	}

	if a != TypeClassInt && a != TypeClassFloat {
		panic(fmt.Sprintf("invalid type class: %v", a))
	}
	if b != TypeClassInt && b != TypeClassFloat {
		panic(fmt.Sprintf("invalid type class: %v", b))
	}

	if a != b {
		return TypeClassMixed
	}
	return a
}

func (d *dwarfReader) getTypeInfo(t dwarf.Type) (typeInfo, error) {
	var ti typeInfo

	switch tt := t.(type) {
	case *dwarf.PtrType, *dwarf.BoolType, *dwarf.FuncType,
		*dwarf.IntType, *dwarf.UintType:
		sz := uint64(tt.Size())
		ti = typeInfo{
			size:  sz,
			align: sz,
			class: TypeClassInt,
			nVars: 1,
		}

	case *dwarf.TypedefType:
		return d.getTypeInfo(tt.Type)
	case *dwarf.FloatType:
		sz := uint64(tt.Size())
		ti = typeInfo{
			size:  sz,
			align: sz,
			class: TypeClassFloat,
			nVars: 1,
		}

	case *dwarf.StructType:
		ti.class = TypeClassNone
		for _, f := range tt.Field {
			sub, err := d.getTypeInfo(f.Type)
			if err != nil {
				return typeInfo{}, err
			}
			// Size must cover the highest field end.
			end := uint64(f.ByteOffset) + sub.size
			ti.size = max(ti.size, end)
			ti.align = max(ti.align, sub.align)
			ti.nVars += sub.nVars
			ti.class = combineTypeClasses(ti.class, sub.class)
		}
		if ti.class == TypeClassNone {
			ti.class = TypeClassMixed
		}

	default:
		return typeInfo{}, fmt.Errorf("unsupported dwarf.Type %T", tt)
	}

	if ti.align == 0 {
		ti.align = ti.size
	}
	return ti, nil
}

// GetTypeInfo converts a DIE offset into size/align/class/var-count information
// using the high-level *dwarf.Type API (no reader rewinds).
func (d *dwarfReader) GetTypeInfo(off dwarf.Offset) (typeInfo, error) {
	if ti, ok := d.typeCache[off]; ok {
		return ti, nil
	}
	t, err := d.Data.Type(off)
	if err != nil {
		return typeInfo{}, err
	}

	ti, err := d.getTypeInfo(t)
	d.typeCache[off] = ti
	return ti, err
}

func (d dwarfReader) GoFuncFieldArgs(fn string) (map[string]FuncFieldArg, error) {
	log.Printf("Searching for function %s in DWARF data\n", fn)
	abi, err := d.DetectSourceABI()
	if err != nil {
		return nil, err
	}
	d.argTracker = NewArgTracker(abi, d.Reader.AddressSize())
	// Reset the reader to the start of the DWARF data.
	d.Reader.Seek(0)
	if !d.GoToEntry(dwarf.TagSubprogram, fn) {
		return nil, fmt.Errorf("function %q not found", fn)
	}

	funcFields := make(map[string]FuncFieldArg)
	argNames := []string{}
	var entry *dwarf.Entry
	entry, err = d.Reader.Next()
	for !errors.Is(err, io.EOF) && entry != nil &&
		entry.Tag != 0 && entry.Tag == dwarf.TagFormalParameter {

		name := entry.Val(dwarf.AttrName).(string)

		if slices.Contains(argNames, name) {
			// Skip this entry as DWARF can generate duplicate entries
			log.Printf("Skipping duplicate entry for %s in function %s\n", name, fn)
			entry, err = d.Reader.Next()
			continue
		}

		argNames = append(argNames, name)
		typeDie, ok := d.Field(entry, dwarf.AttrType)
		if !ok {
			return nil, fmt.Errorf("failed to get type attribute for %s: %w", name, ErrDWARFEntry)
		}
		typeOffset, ok := typeDie.Val.(dwarf.Offset)
		if !ok {
			return nil, fmt.Errorf("type attribute is not a valid dwarf.Offset for %s: %w", name, ErrDWARFEntry)
		}
		ti, err := d.GetTypeInfo(typeOffset)

		if err != nil {
			return nil, fmt.Errorf("failed to get type info for %s: %w", name, err)
		}

		isRetArg := d.IsRetArg(entry)

		funcField, err := d.argTracker.PopLocation(ti.class, ti.size, ti.align, ti.nVars, isRetArg)
		if err != nil {
			return nil, fmt.Errorf("failed to get location for %s: %w", name, err)
		}
		funcFields[name] = funcField
		entry, err = d.Reader.Next()
	}
	return funcFields, nil
}

// GoStructField returns the offset value of a Go struct field. If the struct
// field cannot be found -1 and a non-nil error will be returned.
func (d dwarfReader) GoStructField(id structfield.ID) (int64, error) {
	strct := fmt.Sprintf("%s.%s", id.PkgPath, id.Struct)
	if !d.GoToEntry(dwarf.TagStructType, strct) {
		return -1, fmt.Errorf("struct %q not found", strct)
	}

	e, err := d.EntryInChildren(dwarf.TagMember, id.Field)
	if err != nil {
		return -1, fmt.Errorf("struct field %q not found: %w", id.Field, err)
	}

	f, ok := d.Field(e, dwarf.AttrDataMemberLoc)
	if !ok {
		return -1, fmt.Errorf("struct field offset not found: %w", err)
	}

	v, ok := f.Val.(int64)
	if !ok {
		return -1, errors.New("invalid struct field offset")
	}
	return v, nil
}

// GoToEntry reads until the entry with a tag equal to name is found. True is
// returned if the entry is found, otherwise false is returned.
func (d dwarfReader) GoToEntry(tag dwarf.Tag, name string) bool {
	_, err := d.Entry(tag, name)
	return err == nil
}

func (d dwarfReader) EntriesWithTag(tag dwarf.Tag) ([]*dwarf.Entry, error) {
	entries := make([]*dwarf.Entry, 0)
	for {
		entry, err := d.Reader.Next()
		if errors.Is(err, io.EOF) || entry == nil {
			break
		}

		if entry.Tag == tag {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// Entry returns the entry with a tag equal to name. ErrDWARFEntry is returned
// if the entry cannot be found.
func (d dwarfReader) Entry(tag dwarf.Tag, name string) (*dwarf.Entry, error) {
	for {
		entry, err := d.Reader.Next()
		if errors.Is(err, io.EOF) || entry == nil {
			break
		}

		if entry.Tag == tag {
			if f, ok := d.Field(entry, dwarf.AttrName); ok {
				if name == f.Val.(string) {
					return entry, nil
				}
			}
		}
	}
	return nil, ErrDWARFEntry
}

// EntryInChildren returns the entry with a tag equal to name within the
// children of the current entry. ErrDWARFEntry is returned if the entry cannot
// be found.
func (d dwarfReader) EntryInChildren(tag dwarf.Tag, name string) (*dwarf.Entry, error) {
	for {
		entry, err := d.Reader.Next()
		if errors.Is(err, io.EOF) || entry == nil || entry.Tag == 0 {
			break
		}

		if entry.Tag == tag {
			if f, ok := d.Field(entry, dwarf.AttrName); ok {
				if name == f.Val.(string) {
					return entry, nil
				}
			}
		}
	}
	return nil, ErrDWARFEntry
}

// Field returns the field from the entry e that has attribute a and true.
// If no field is found, an empty field is returned with false.
func (d dwarfReader) Field(e *dwarf.Entry, a dwarf.Attr) (dwarf.Field, bool) {
	for _, f := range e.Field {
		if f.Attr == a {
			return f, true
		}
	}
	return dwarf.Field{}, false
}
