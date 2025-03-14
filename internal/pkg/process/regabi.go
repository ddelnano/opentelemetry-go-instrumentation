package process

type golangRegABI struct {
	// We’ll track how many register “slots” are left for integer args, float args, etc.
	// For simplicity, assume 8 integer registers, 8 floating registers, etc.
	intRegsLeft []RegisterName
	fpRegsLeft  []RegisterName
	// Offsets used if we run out of registers or the argument is large
	stackOffset int64
	// Let’s also track offset counters if we differentiate arg vs. return regs
	// (some compilers use the same or partially overlapping sets).
	intRetRegsLeft []RegisterName
	fpRetRegsLeft  []RegisterName
	retStackOffset int64 // If we have “return parameters” that exceed register capacity
}

// Initialize a new Golang RegABI model for x86_64, somewhat like Pixie’s approach.
func newGolangRegABI() *golangRegABI {
	return &golangRegABI{
		intRegsLeft: []RegisterName{
			kRAX, kRBX, kRCX, kRDI, kRSI, kR8, kR9, kR10, kR11,
		},
		fpRegsLeft: []RegisterName{
			kXMM0, kXMM1, kXMM2, kXMM3, kXMM4, kXMM5, kXMM6, kXMM7, kXMM8, kXMM9,
			kXMM10, kXMM11, kXMM12, kXMM13, kXMM14,
		},
		// same for return arguments, can be specialized or the same:
		intRetRegsLeft: []RegisterName{
			// Some compilers might reuse RAX, RDX, etc. This is a simplification:
			kRAX, kRBX, kRCX, kRDI, kRSI, kR8, kR9, kR10, kR11,
		},
		fpRetRegsLeft: []RegisterName{
			kXMM0, kXMM1, kXMM2, kXMM3, kXMM4, kXMM5, kXMM6, kXMM7, kXMM8, kXMM9,
			kXMM10, kXMM11, kXMM12, kXMM13, kXMM14,
		},

		// Start stack offset at 0. We’ll push upward as we place arguments on the stack.
		stackOffset:    0,
		retStackOffset: 0,
	}
}

func (m *golangRegABI) popLocation(tclass TypeClass, size, alignment uint64, numVars int, isRetArg bool) (VarLocation, error) {
	// Align the stack offset to the argument alignment if we eventually put it on the stack:
	align := func(off int64, align uint64) int64 {
		a := int64(align)
		// round up off to multiple of a
		remainder := off % a
		if remainder == 0 {
			return off
		}
		return off + (a - remainder)
	}

	loc := VarLocation{}

	// In real usage, you’d detect “pointer to a large struct,” etc. This example is simplified:
	switch tclass {
	case Integer:
		// If it’s a return arg, use ret registers.
		// Otherwise, use intRegsLeft. If none left, place on stack.
		if isRetArg {
			if numVars <= len(m.intRetRegsLeft) {
				loc.LocType = Register
				loc.Regs = m.intRetRegsLeft[:numVars]
				m.intRetRegsLeft = m.intRetRegsLeft[numVars:]
				loc.Offset = 0 // We can store an offset that increments with each register
			} else {
				// Not enough registers left, place on stack.
				m.retStackOffset = align(m.retStackOffset, alignment)
				loc.LocType = Stack
				loc.Offset = m.retStackOffset
				m.retStackOffset += int64(size)
			}
		} else {
			if numVars <= len(m.intRegsLeft) {
				loc.LocType = Register
				loc.Regs = m.intRegsLeft[:numVars]
				m.intRegsLeft = m.intRegsLeft[numVars:]
				loc.Offset = 0
			} else {
				m.stackOffset = align(m.stackOffset, alignment)
				loc.LocType = Stack
				loc.Offset = m.stackOffset
				m.stackOffset += int64(size)
			}
		}
	case Float:
		if isRetArg {
			if numVars <= len(m.fpRetRegsLeft) {
				loc.LocType = RegisterFP
				loc.Regs = m.fpRetRegsLeft[:numVars]
				m.fpRetRegsLeft = m.fpRetRegsLeft[numVars:]
				loc.Offset = 0
			} else {
				m.retStackOffset = align(m.retStackOffset, alignment)
				loc.LocType = Stack
				loc.Offset = m.retStackOffset
				m.retStackOffset += int64(size)
			}
		} else {
			if numVars <= len(m.fpRegsLeft) {
				loc.LocType = RegisterFP
				loc.Regs = m.fpRegsLeft[:numVars]
				m.fpRegsLeft = m.fpRegsLeft[numVars:]
				loc.Offset = 0
			} else {
				m.stackOffset = align(m.stackOffset, alignment)
				loc.LocType = Stack
				loc.Offset = m.stackOffset
				m.stackOffset += int64(size)
			}
		}
	case Mixed:
		// A more complicated struct containing both float and integer fields.
		// The real Go compiler has complicated rules. We just place on stack to keep it simple:
		m.stackOffset = align(m.stackOffset, alignment)
		loc.LocType = Stack
		loc.Offset = m.stackOffset
		m.stackOffset += int64(size)
	default:
		// fallback for pointer, arrays, unknown. Often these are treated as “Integer” for x86_64.
		// We could do special logic for pointer if you prefer:
		if isRetArg {
			if numVars <= len(m.intRetRegsLeft) {
				loc.LocType = Register
				loc.Regs = m.intRetRegsLeft[:numVars]
				m.intRetRegsLeft = m.intRetRegsLeft[numVars:]
				loc.Offset = 0
			} else {
				m.retStackOffset = align(m.retStackOffset, alignment)
				loc.LocType = Stack
				loc.Offset = m.retStackOffset
				m.retStackOffset += int64(size)
			}
		} else {
			if numVars <= len(m.intRegsLeft) {
				loc.LocType = Register
				loc.Regs = m.intRegsLeft[:numVars]
				m.intRegsLeft = m.intRegsLeft[numVars:]
				loc.Offset = 0
			} else {
				m.stackOffset = align(m.stackOffset, alignment)
				loc.LocType = Stack
				loc.Offset = m.stackOffset
				m.stackOffset += int64(size)
			}
		}
	}
	return loc, nil
}
