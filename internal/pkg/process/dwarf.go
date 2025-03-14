// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package process

import (
	"debug/dwarf"
	"errors"
	"fmt"
	"io"

	"go.opentelemetry.io/auto/internal/pkg/funcfield"
	"go.opentelemetry.io/auto/internal/pkg/structfield"
)

// ErrDWARFEntry is returned if an entry is not found within DWARF data.
var ErrDWARFEntry = errors.New("DWARF entry not found")

// DWARF provides convenience in accessing DWARF debugging data.
type DWARF struct {
	Data   *dwarf.Data
	Reader *dwarf.Reader
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

// enum VarType
type VarType int

const (
	Unspecified VarType = iota
	Void
	BaseType
	Pointer
	Class
	Struct
	Subroutine
)

type RegisterName int

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

type TypeInfo struct {
	VarType  VarType
	TypeName string
	DeclType string
}

// VarLocation holds location information for function arguments.
type VarLocation struct {
	LocType LocationType
	Offset  int64
	Regs    []RegisterName
}

// LocationType represents where an argument is stored.
type LocationType int

const (
	Unknown LocationType = iota
	Stack
	StackBP
	Register
	RegisterFP
)

type TypeClass int

const (
	None TypeClass = iota
	Integer
	Float
	Mixed
)

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

// // ArgInfo stores information about function arguments, including location and type.
type ArgInfo struct {
	TypeInfo TypeInfo
	Location VarLocation
	RetArg   bool // True if the argument is actually a return value
}

// GetParamDIEs re-seeks the DWARF data to fnDie.Offset,
// reads the children until Tag==0, and returns any with TagFormalParameter.
func GetParamDIEs(fnDie *dwarf.Entry, data *dwarf.Data) ([]*dwarf.Entry, error) {
	reader := data.Reader()
	reader.Seek(fnDie.Offset) // Move to this function DIE's offset

	var params []*dwarf.Entry
	for {
		entry, err := reader.Next()
		if err != nil {
			return nil, fmt.Errorf("error reading DWARF: %w", err)
		}
		if entry == nil || entry.Tag == 0 {
			// Tag == 0 => reached the end of *this* DIE's children.
			break
		}
		if entry.Tag == dwarf.TagFormalParameter {
			params = append(params, entry)
		}
	}
	return params, nil
}

func (d DWARF) GoFunctionArguments(id funcfield.ID) (map[string]ArgInfo, error) {
	// 1) Find the subprogram DIE with name == id.Name.
	d.Reader.Seek(0)
	fnDie, err := d.Entry(dwarf.TagSubprogram, id.Name)
	if err != nil {
		return nil, fmt.Errorf("could not find subprogram %q: %w", id.Name, err)
	}

	// 2) Collect all TagFormalParameter DIEs for *this* function.
	paramDIEs, err := GetParamDIEs(fnDie, d.Data)
	if err != nil {
		return nil, fmt.Errorf("GetParamDIEs failed for %q: %w", id.Name, err)
	}

	// 3) Build the simplified Go RegABI model.
	model := newGolangRegABI()

	argsMap := make(map[string]ArgInfo)

	// 4) Parse each parameter DIE.
	for _, paramDie := range paramDIEs {
		// Param name
		field, ok := d.Field(paramDie, dwarf.AttrName)
		if !ok {
			// Sometimes compiler may omit names for generated parameters
			continue
		}
		pname, _ := field.Val.(string)
		if pname == "" {
			continue
		}

		// 4a) Get type info (VarType, TypeClass, sizes, etc.).
		ti, tclass, size, align, nvars, err := d.getTypeInfo(paramDie)
		if err != nil {
			// skip or log
			continue
		}

		// 4b) Detect if it's a "return param" (for most Go builds, we can't rely on that,
		//     so let's assume false or implement your own logic).
		isRet, _ := d.isGoReturnParam(paramDie)

		// 4c) Ask the ABI model where to locate this param.
		fmt.Printf("tclass %v size %v align %v nvars %v\n", tclass, size, align, nvars)
		loc, err := model.popLocation(tclass, size, align, nvars, isRet)
		if err != nil {
			// skip or log
			continue
		}

		argsMap[pname] = ArgInfo{
			TypeInfo: ti,
			Location: loc,
			RetArg:   isRet,
		}
	}

	return argsMap, nil
}

func (d DWARF) resolveTypeDie(die *dwarf.Entry) (*dwarf.Entry, error) {
	// We want to follow DW_AT_type. Then if that type is a typedef or const, follow again, etc.
	tfield, ok := d.Field(die, dwarf.AttrType)
	if !ok {
		// No type? Might be "void" or something with no type.
		return nil, nil
	}
	offset, ok := tfield.Val.(dwarf.Offset)
	if !ok {
		return nil, fmt.Errorf("type attribute was not a dwarf.Offset")
	}

	// We can use Reader.Seek() with that offset, then Reader.Next() to get the pointed-to DIE.
	// Then we see if that DIE is typedef/pointer/const, etc., so we can chase further if needed.
	// (In large DWARF, you'd want a more robust approach or a caching mechanism.)
	d.Reader.Seek(offset)
	td, err := d.Reader.Next()
	if err != nil {
		return nil, fmt.Errorf("failed to read type DIE at offset %v: %w", offset, err)
	}
	return td, nil
}

// This function recurses until it hits a "base" or "pointer" or "struct" etc.
func (d DWARF) chaseTypedefs(e *dwarf.Entry) (*dwarf.Entry, error) {
	if e == nil {
		return e, nil
	}
	for {
		switch e.Tag {
		case dwarf.TagTypedef, dwarf.TagConstType:
			// Keep chasing
			td, err := d.resolveTypeDie(e)
			if err != nil {
				return nil, err
			}
			if td == nil {
				return nil, nil
			}
			e = td // loop again
		default:
			// Stop
			return e, nil
		}
	}
}

func mapDieTagToVarType(tag dwarf.Tag) VarType {
	switch tag {
	case dwarf.TagPointerType:
		return Pointer
	case dwarf.TagBaseType:
		return BaseType
	case dwarf.TagClassType:
		return Class
	case dwarf.TagStructType:
		return Struct
	case dwarf.TagSubroutineType:
		return Subroutine
	default:
		return Unspecified
	}
}

func (d DWARF) getTypeInfo(paramEntry *dwarf.Entry) (TypeInfo, TypeClass, uint64, uint64, int, error) {
	// 1) chase to underlying type
	underlying, err := d.resolveTypeDie(paramEntry)
	if err != nil {
		return TypeInfo{}, None, 0, 0, 0, err
	}
	underlying, err = d.chaseTypedefs(underlying)
	if err != nil {
		return TypeInfo{}, None, 0, 0, 0, err
	}

	if underlying == nil {
		// treat as void
		return TypeInfo{VarType: Void, TypeName: "", DeclType: ""}, None, 0, 0, 0, nil
	}

	vtype := mapDieTagToVarType(underlying.Tag)

	// We can attempt to read the name from dwarf.AttrName or fallback
	tfield, _ := d.Field(underlying, dwarf.AttrName)
	typeName, _ := tfield.Val.(string)

	// For simplicity, we treat all pointers as "Integer" for the ABI planner.
	// If it’s a base type with dwarf.AttrEncoding == float, we do Float, else Integer, etc.
	// Here’s a minimal approach:
	var tclass TypeClass = Integer
	if vtype == Pointer {
		tclass = Integer
	} else if vtype == BaseType {
		// Possibly read DW_AT_encoding:
		// For demonstration, assume any "float" substring means float (or check DW_ATE_float).
		// In real code, parse the encoding attribute carefully.
		if typeName == "float32" || typeName == "float64" {
			tclass = Float
		} else {
			tclass = Integer
		}
	} else if vtype == Struct {
		// For Go slices, strings, or interface, we might treat them as “Struct” or “Mixed”.
		// You’d do a deeper pass to see if it’s purely integer fields or if it has float fields.
		// Here we assume it’s integer-like, so tclass=Integer, or we do something like:
		// tclass = Mixed
		// but let’s guess if it’s a slice (like "[]uint8"), treat it as integer-based:
		if len(typeName) > 2 && typeName[:2] == "[]" {
			tclass = Integer // or Mixed if you want
		} else {
			// real usage: you’d parse sub-members
			tclass = Mixed
		}
	}

	// 2) figure out size/alignment. A real approach calls dwarf's Type.Size() or parse DW_AT_byte_size etc.
	// For demonstration, let's do quick heuristics:
	size := uint64(8)      // default 8 bytes
	alignment := uint64(8) // default alignment
	numVars := 1           // how many 8-byte slots, for the “RegABI” approach
	if vtype == Pointer {
		size = 8
		alignment = 8
		numVars = 1
	} else if vtype == BaseType {
		// We might do: if it’s “int32,” size=4, alignment=4. If it’s “int64,” size=8, ...
		// Hard-code a couple of examples:
		if typeName == "int32" {
			size, alignment, numVars = 4, 4, 1
		} else if typeName == "int64" {
			size, alignment, numVars = 8, 8, 1
		}
		// etc.
	} else if vtype == Struct {
		// For a slice, it’s typically 3 words in Go (ptr, len, cap),
		// each 8 bytes on 64-bit. So total 24 bytes. We’ll treat that as 3 8-byte slots:
		if len(typeName) > 2 && typeName[:2] == "[]" {
			size = 24
			alignment = 8
			numVars = 3
		} else {
			// fallback
			size = 16
			alignment = 8
			numVars = 2
		}
	}

	ti := TypeInfo{
		VarType:  vtype,
		TypeName: typeName,
		DeclType: typeName, // we skip more advanced typedef expansions
	}
	return ti, tclass, size, alignment, numVars, nil
}

func (d DWARF) isGoReturnParam(e *dwarf.Entry) (bool, error) {
	// for _, f := range e.Field {
	// 	if f.Attr == dwarf.AttrVariableParameter {
	// 		// Some compilers store it as a boolean or 1/0
	// 		if val, ok := f.Val.(int64); ok && val == 1 {
	// 			return true, nil
	// 		}
	// 		// If you see `DW_FORM_flag_present` or boolean, parse accordingly.
	// 		return true, nil
	// 	}
	// }
	return false, nil
}
