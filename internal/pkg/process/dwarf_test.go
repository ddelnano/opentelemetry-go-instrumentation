package process

import (
	"debug/elf"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/auto/internal/pkg/funcfield"
)

func TestDWARFFunctionOffsets(t *testing.T) {

	// Path to the compiled test binary containing the function.
	binaryPath := "testdata/client"

	// Open the ELF binary.
	file, err := os.Open(binaryPath)
	assert.NoError(t, err, "Failed to open test binary")
	defer file.Close()

	// Parse the ELF binary.
	elfFile, err := elf.NewFile(file)
	assert.NoError(t, err, "Failed to parse ELF file")

	// Extract DWARF data from the ELF binary.
	dwarfData, err := elfFile.DWARF()
	dwarfParser := DWARF{Data: dwarfData, Reader: dwarfData.Reader()} // Define the function ID for `crypto/tls.(*Conn).Read`.

	args, err := dwarfParser.GoFunctionArguments(funcfield.ID{
		Name:    "crypto/tls.(*Conn).Read",
		Args:    []string{"c", "b"},
		Retvals: []int{0, 1},
	})

	if err != nil {
		t.Fatalf("Failed to get function arguments: %v", err)
	}
	fmt.Printf("Function arguments: %+v\n", args)

	if len(args) == 0 {
		t.Fatalf("Function arguments not found")
	}

	if _, ok := args["c"]; !ok {
		t.Fatalf("Argument 'c' not found")
	}

	if _, ok := args["b"]; !ok {
		t.Fatalf("Argument 'b' not found")
	}

	if _, ok := args["~r0"]; !ok {
		t.Fatalf("Argument 'retval0' not found")
	}

	if _, ok := args["~r1"]; !ok {
		t.Fatalf("Argument 'retval0' not found")
	}

	expectedArgC := ArgInfo{
		TypeInfo: TypeInfo{
			VarType:  Pointer,
			TypeName: "*crypto/tls.Conn",
			DeclType: "*crypto/tls.Conn",
		},
		Location: VarLocation{
			LocType: Register,
			Offset:  0,
			Regs: []RegisterName{
				kRAX,
			},
		},
		RetArg: false,
	}

	argC := args["c"]
	assert.Equal(t, expectedArgC, argC, "Argument 'c' does not match expected value")

	expectedArgB := ArgInfo{
		TypeInfo: TypeInfo{
			VarType:  Struct,
			TypeName: "[]uint8",
			DeclType: "[]uint8",
		},
		Location: VarLocation{
			LocType: Register,
			Offset:  8,
			Regs: []RegisterName{
				kRBX,
				kRCX,
				kRDI,
			},
		},
		RetArg: false,
	}
	argB := args["b"]
	assert.Equal(t, expectedArgB, argB, "Argument 'b' does not match expected value")
	// I20250314 14:41:45.260408    12 dwarf_reader.cc:958] crypto/tls.(*Conn).Write arg: ~r0
	// I20250314 14:41:45.260432    12 dwarf_reader.cc:982] type_info=[type=kBaseType decl_type=int type_name=int] location=[type=kRegister offset=0 registers=[kRAX]] retarg=true
	// I20250314 14:41:45.260442    12 dwarf_reader.cc:958] crypto/tls.(*Conn).Write arg: ~r0
	// I20250314 14:41:45.260448    12 dwarf_reader.cc:958] crypto/tls.(*Conn).Write arg: ~r1
	// I20250314 14:41:45.260504    12 dwarf_reader.cc:982] type_info=[type=kStruct decl_type=error type_name=runtime.iface] location=[type=kRegister offset=8 registers=[kRBX,kRCX]] retarg=true

	expectedR0 := ArgInfo{
		TypeInfo: TypeInfo{
			VarType:  BaseType,
			TypeName: "int",
			DeclType: "int",
		},
		Location: VarLocation{
			LocType: Register,
			Offset:  0,
			Regs: []RegisterName{
				kRAX,
			},
		},
		RetArg: true,
	}
	r0 := args["~r0"]
	assert.Equal(t, expectedR0, r0, "Expected ~r0 to be %v, but got %v", expectedR0, r0)

	expectedR1 := ArgInfo{
		TypeInfo: TypeInfo{
			VarType:  Struct,
			TypeName: "runtime.iface",
			DeclType: "error",
		},
		Location: VarLocation{
			LocType: Register,
			Offset:  8,
			Regs: []RegisterName{
				kRBX,
				kRCX,
			},
		},
		RetArg: true,
	}
	r1 := args["~r1"]
	assert.Equal(t, expectedR1, r1, "Expected ~r1 to be %v, but got %v", expectedR1, r1)
}
