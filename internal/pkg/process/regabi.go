package process

type golangRegABI struct {
	// The available registers:
	intRegsLeft []RegisterName
	fpRegsLeft  []RegisterName

	// The running offset for integer-based arguments that land in registers.
	// Each register is treated as an 8-byte slot in a “register region.”
	currentIntRegOffset int64

	// Same idea for floating-point arguments, if you ever have float param offset checking.
	currentFpRegOffset int64

	// The running offset for stack-based arguments.
	stackOffset int64
}

func newGolangRegABI() *golangRegABI {
	return &golangRegABI{
		intRegsLeft: []RegisterName{
			kRAX, kRBX, kRCX, kRDI, kRSI, kR8, kR9, kR10, kR11,
		},
		fpRegsLeft: []RegisterName{
			kXMM0, kXMM1, kXMM2, kXMM3, kXMM4, kXMM5, kXMM6, kXMM7,
			kXMM8, kXMM9, kXMM10, kXMM11, kXMM12, kXMM13, kXMM14,
		},
		currentIntRegOffset: 0,
		currentFpRegOffset:  0,
		stackOffset:         0,
	}
}

func (m *golangRegABI) popLocation(
	tclass TypeClass,
	size, alignment uint64,
	numVars int,
	isRetArg bool,
) (VarLocation, error) {

	// align() is used only for stack usage
	align := func(off int64, a uint64) int64 {
		r := off % int64(a)
		if r == 0 {
			return off
		}
		return off + (int64(a) - r)
	}

	loc := VarLocation{}

	switch tclass {
	case Integer:
		// If we have enough leftover integer registers, allocate them:
		if numVars <= len(m.intRegsLeft) {
			loc.LocType = Register
			loc.Regs = m.intRegsLeft[:numVars]
			m.intRegsLeft = m.intRegsLeft[numVars:]

			// Here we do an offset in "register space" if your test requires it
			loc.Offset = m.currentIntRegOffset
			m.currentIntRegOffset += int64(numVars * 8)

		} else {
			// Not enough registers => place on the stack
			m.stackOffset = align(m.stackOffset, alignment)
			loc.LocType = Stack
			loc.Offset = m.stackOffset
			m.stackOffset += int64(size)
		}

	case Float:
		// Similar to integer, but separate register list and offset
		if numVars <= len(m.fpRegsLeft) {
			loc.LocType = RegisterFP
			loc.Regs = m.fpRegsLeft[:numVars]
			m.fpRegsLeft = m.fpRegsLeft[numVars:]

			loc.Offset = m.currentFpRegOffset
			m.currentFpRegOffset += int64(numVars * 8)

		} else {
			m.stackOffset = align(m.stackOffset, alignment)
			loc.LocType = Stack
			loc.Offset = m.stackOffset
			m.stackOffset += int64(size)
		}

	case Mixed:
		// For complicated structs with both float and integer subfields,
		// you might put them partially in registers or on the stack.
		// As a simple approach:
		m.stackOffset = align(m.stackOffset, alignment)
		loc.LocType = Stack
		loc.Offset = m.stackOffset
		m.stackOffset += int64(size)

	default: // covers None or anything else
		// fallback => stack
		m.stackOffset = align(m.stackOffset, alignment)
		loc.LocType = Stack
		loc.Offset = m.stackOffset
		m.stackOffset += int64(size)
	}

	return loc, nil
}
