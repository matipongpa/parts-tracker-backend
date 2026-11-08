package parts

import "testing"

func TestClassifyDesignator(t *testing.T) {
	cases := []struct {
		in   string
		want PartType
	}{
		{"C12", PartTypeCapacitor},
		{" r3 ", PartTypeResistor},
		{"R3,R4", PartTypeResistor},
		{"RN1", PartTypeResistorNetwork},
		{"ZD7", PartTypeZenerDiode},
		{"U1", PartTypeIntegratedCircuit},
		{"", PartTypeUnknown},
		{"ZZ9", PartTypeUnknown},
	}
	for _, tc := range cases {
		if got := classifyDesignator(tc.in); got != tc.want {
			t.Errorf("classifyDesignator(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
