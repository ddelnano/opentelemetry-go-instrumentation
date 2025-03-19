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
func NewArgTracker(abi ABI) ArgTracker {
	if abi == AbiGoReg {
		return &GoRegABIArgTracker{
			CurrentStackOffset: 0,
			IntArgRegs:         []Registers{kRAX, kRBX, kRCX, kRDX, kRSI, kR8, kR9, kR10, kR11},
			IntRetValRegs:      []Registers{kRAX, kRBX, kRCX, kRDX, kRSI, kR8, kR9, kR10, kR11},
			FloatArgRegs:       []Registers{kXMM0, kXMM1, kXMM2, kXMM3, kXMM4, kXMM5, kXMM6, kXMM7, kXMM8, kXMM9, kXMM10, kXMM11, kXMM12, kXMM13, kXMM14},
			FloatRetValRegs:    []Registers{kXMM0, kXMM1, kXMM2, kXMM3, kXMM4, kXMM5, kXMM6, kXMM7, kXMM8, kXMM9, kXMM10, kXMM11, kXMM12, kXMM13, kXMM14},
		}
	} else if abi == AbiGoStack {
		return &GoStackABIArgTracker{
			CurrentStackOffset: 0,
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
	// TODO(ddelnano): This should be read from the binary. This works for 64 bit binaries until then.
	var regSize int = 8
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

		// TODO(ddelnano): Add registers to funcFieldArg
		// Pop a register off for each variable in the type.
		for i := 0; i < numVars; i++ {
			*registers = (*registers)[1:]
		}
		*regOffset += uint64(numVars * regSize)
		return funcFieldArg, nil
	} else {
		funcFieldArg.Location = funcfield.Stack
		return funcFieldArg, errors.New("Stack arguments not implemented for GoRegABIArgTracker")
	}
}

type GoStackABIArgTracker struct {
	CurrentStackOffset uint64
}

func IntRoundDivide(x uint64, y uint64) uint64 {
	return (x + (y - 1)) / y
}

func SnapUpToMultiple(x, y uint64) uint64 {
	return IntRoundDivide(x, y) * y
}

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

// DWARF provides convenience in accessing DWARF debugging data.
type DWARF struct {
	Reader     *dwarf.Reader
	argTracker ArgTracker
}

type FuncFieldArg struct {
	Offset   uint64             `json:"offset"`
	Location funcfield.Location `json:"location"`
	RetArg   bool               `json:"ret_arg"` // true if this is a return argument
}

// TODO(ddelnano): Reading the go .buildinfo might be easier and would be significantly faster
func (d DWARF) DetectSourceABI() (ABI, error) {
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

func (d DWARF) IsRetArg(die *dwarf.Entry) bool {
	if f, ok := d.Field(die, dwarf.AttrVarParam); ok {
		varParam := f.Val.(bool)
		return varParam
	}
	return false
}

func (d DWARF) GetTypeDIE(entry *dwarf.Entry) (*dwarf.Entry, error) {
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

func (d DWARF) GetTypeClass(entry *dwarf.Entry) (TypeClass, error) {
	switch entry.Tag {
	case dwarf.TagPointerType, dwarf.TagSubroutineType:
		return TypeClassInt, nil
	case dwarf.TagBaseType:
		field, ok := d.Field(entry, dwarf.AttrEncoding)
		if !ok {
			return TypeClassNone, fmt.Errorf("failed to get encoding attribute: %w", ErrDWARFEntry)
		}
		encoding, ok := field.Val.(int64)
		if !ok {
			return TypeClassNone, fmt.Errorf("encoding attribute is not a valid string: %w", ErrDWARFEntry)
		}
		// TODO(ddelnano): Determine how the less common float types should be handled (DW_ATE_complex_float, DW_ATE_imaginary_float, etc.)
		// 0x04 == DW_ATE_float
		if encoding == 0x04 {
			return TypeClassFloat, nil
		}
		return TypeClassInt, nil
	case dwarf.TagStructType:
		structTypeClass := TypeClassNone
		for {
			die, err := d.Reader.Next()
			if errors.Is(err, io.EOF) || die == nil || die.Tag == 0 {
				return structTypeClass, nil
			}
			if err != nil {
				return 0, fmt.Errorf("error reading struct members: %w", err)
			}
			if die.Tag == dwarf.TagMember {
				offset := die.Offset
				typeDie, err := d.GetTypeDIE(die)
				if err != nil {
					return 0, fmt.Errorf("error getting type DIE for member: %w", err)
				}
				typeClass, err := d.GetTypeClass(typeDie)
				if err != nil {
					return 0, fmt.Errorf("error getting alignment size for member: %w", err)
				}
				d.Reader.Seek(offset)
				d.Reader.Next()
				structTypeClass = combineTypeClasses(structTypeClass, typeClass)
			}
		}
		return TypeClassMixed, nil
	default:
		return TypeClassNone, fmt.Errorf("unsupported tag for type class: %v", entry.Tag)
	}
}

func (d DWARF) GetBaseOrStructTypeByteSize(entry *dwarf.Entry) (uint64, error) {
	field, ok := d.Field(entry, dwarf.AttrByteSize)
	if !ok {
		return 0, fmt.Errorf("failed to get byte size attribute: %w", ErrDWARFEntry)
	}
	byteSize, ok := field.Val.(int64)
	if !ok {
		return 0, fmt.Errorf("byte size attribute is not a valid int64: %w", ErrDWARFEntry)
	}
	return uint64(byteSize), nil
}

func (d DWARF) GetTypeByteSize(entry *dwarf.Entry) (uint64, error) {
	switch entry.Tag {
	case dwarf.TagPointerType, dwarf.TagSubroutineType:
		// TODO(ddelnano): This should be read from DWARF
		return 8, nil // Assuming 64-bit pointers for simplicity
	case dwarf.TagBaseType, dwarf.TagStructType:
		return d.GetBaseOrStructTypeByteSize(entry)
	default:
		return 0, fmt.Errorf("unsupported tag for byte size: %v %v", entry.Tag, entry)
	}
}

func (d DWARF) GetAlignmentSize(entry *dwarf.Entry) (uint64, error) {
	switch entry.Tag {
	case dwarf.TagPointerType, dwarf.TagSubroutineType:
		// TODO(ddelnano): This should be read from DWARF
		return 8, nil // Assuming 64-bit pointers for simplicity
	case dwarf.TagBaseType:
		return d.GetBaseOrStructTypeByteSize(entry)
	case dwarf.TagStructType:
		maxSize := uint64(0)
		for {
			die, err := d.Reader.Next()
			if errors.Is(err, io.EOF) || die == nil || die.Tag == 0 {
				return maxSize, nil
			}
			if err != nil {
				return 0, fmt.Errorf("error reading struct members: %w", err)
			}
			if die.Tag == dwarf.TagMember {
				offset := die.Offset
				typeDie, err := d.GetTypeDIE(die)
				if err != nil {
					return 0, fmt.Errorf("error getting type DIE for member: %w", err)
				}
				size, err := d.GetAlignmentSize(typeDie)
				if err != nil {
					return 0, fmt.Errorf("error getting alignment size for member: %w", err)
				}
				maxSize = max(maxSize, size)
				d.Reader.Seek(offset)
				d.Reader.Next()
			}
		}
		return maxSize, nil
	default:
		return 0, fmt.Errorf("unsupported tag for alignment size: %v %+v", entry.Tag, entry)
	}
}

func (d DWARF) GetNumVars(entry *dwarf.Entry) (int, error) {
	tag := entry.Tag
	switch tag {
	case dwarf.TagPointerType, dwarf.TagSubroutineType, dwarf.TagBaseType:
		return 1, nil
	case dwarf.TagStructType:
		numVars := 0
		for {
			die, err := d.Reader.Next()
			if errors.Is(err, io.EOF) || die == nil || die.Tag == 0 {
				return numVars, nil
			}
			if err != nil {
				return 0, fmt.Errorf("error reading struct members: %w", err)
			}
			if die.Tag == dwarf.TagMember {
				offset := die.Offset
				typeDie, err := d.GetTypeDIE(die)
				if err != nil {
					return 0, fmt.Errorf("error getting type DIE for member: %w", err)
				}
				vars, err := d.GetNumVars(typeDie)
				if err != nil {
					return 0, fmt.Errorf("error getting number of variables for member: %w", err)
				}
				numVars += vars
				// The type DIE is not in order, so we must seek back to the struct member DIE
				d.Reader.Seek(offset)
				d.Reader.Next() // Reset the reader to the member DIE
			}
		}
		return 0, errors.New("struct types are not implemented yet")
	default:
		return 0, errors.New(fmt.Sprintf("unsupported tag: %v", tag))
	}
}

func (d DWARF) GoFuncFieldArgs(fn string) (map[string]FuncFieldArg, error) {
	log.Printf("Searching for function %s in DWARF data\n", fn)
	abi, err := d.DetectSourceABI()
	if err != nil {
		return nil, err
	}
	d.argTracker = NewArgTracker(abi)
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
		isRetArg := d.IsRetArg(entry)
		// TODO(ddelnano): Must get TYPE DIE and then reset
		offset := entry.Offset
		typeDie, err := d.GetTypeDIE(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to get type DIE for %s: %w", name, err)
		}
		typeSize, err := d.GetTypeByteSize(typeDie)
		if err != nil {
			return nil, fmt.Errorf("failed to get type size for %s: %w", name, err)
		}

		typeClass, err := d.GetTypeClass(typeDie)
		if err != nil {
			return nil, fmt.Errorf("failed to get type class for %s: %w", name, err)
		}

		d.Reader.Seek(typeDie.Offset)
		alignmentSize, err := d.GetAlignmentSize(typeDie)
		if err != nil {
			return nil, fmt.Errorf("failed to get alignment size for %s: %w", name, err)
		}

		d.Reader.Seek(typeDie.Offset)
		numVars, err := d.GetNumVars(typeDie)

		// Reset to the original position
		d.Reader.Seek(offset)
		d.Reader.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to get number of variables for %s: %w", name, err)
		}
		funcField, err := d.argTracker.PopLocation(typeClass, typeSize, alignmentSize, numVars, isRetArg)
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
func (d DWARF) GoStructField(id structfield.ID) (int64, error) {
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
func (d DWARF) GoToEntry(tag dwarf.Tag, name string) bool {
	_, err := d.Entry(tag, name)
	return err == nil
}

func (d DWARF) EntriesWithTag(tag dwarf.Tag) ([]*dwarf.Entry, error) {
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
func (d DWARF) Entry(tag dwarf.Tag, name string) (*dwarf.Entry, error) {
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
func (d DWARF) EntryInChildren(tag dwarf.Tag, name string) (*dwarf.Entry, error) {
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
func (d DWARF) Field(e *dwarf.Entry, a dwarf.Attr) (dwarf.Field, bool) {
	for _, f := range e.Field {
		if f.Attr == a {
			return f, true
		}
	}
	return dwarf.Field{}, false
}
