package parts

import (
	"regexp"
	"strings"
)

// PartType is the human-readable component category stored in Part.Type.
type PartType string

const (
	PartTypeAssembly                PartType = "assembly"
	PartTypeIsolator                PartType = "isolator"
	PartTypeBridgeRectifier         PartType = "Bridge-rectifier"
	PartTypeBattery                 PartType = "Battery"
	PartTypeCapacitor               PartType = "Capacitor"
	PartTypeCircuitBreaker          PartType = "Circuit breaker"
	PartTypeCapacitorNetwork        PartType = "Capacitor network"
	PartTypeDiode                   PartType = "Diode"
	PartTypeDelayLine               PartType = "Delay line"
	PartTypeDiodeNetwork            PartType = "Diode network"
	PartTypeDisplay                 PartType = "Display"
	PartTypeFuse                    PartType = "Fuse"
	PartTypeFerriteBead             PartType = "Ferrite bead"
	PartTypeFiducial                PartType = "Fiducial"
	PartTypeFilter                  PartType = "Filter"
	PartTypeGeneratorOscillator     PartType = "Generator/Oscillator"
	PartTypeGraphicalLogo           PartType = "Graphical logo"
	PartTypeGeneralNetwork          PartType = "General network"
	PartTypeHardware                PartType = "Hardware"
	PartTypeCirculator              PartType = "Circulator"
	PartTypeJackConnector           PartType = "Jack connector"
	PartTypeJumper                  PartType = "Jumper"
	PartTypeRelayContactor          PartType = "Relay contactor"
	PartTypeInductor                PartType = "Inductor"
	PartTypeLoudspeakerBuzzer       PartType = "Loudspeaker buzzer"
	PartTypeMotor                   PartType = "Motor"
	PartTypeMicrophone              PartType = "Microphone"
	PartTypeMechanicalPart          PartType = "Mechanical part"
	PartTypeOptoIsolator            PartType = "Opto-isolator"
	PartTypePlugConnector           PartType = "Plug connector"
	PartTypePhotodiode              PartType = "Photodiode"
	PartTypeTransistor              PartType = "Transistor"
	PartTypeResistor                PartType = "Resistor"
	PartTypeResistorNetwork         PartType = "Resistor network"
	PartTypeThermistor              PartType = "Thermistor"
	PartTypeVaristor                PartType = "Varistor"
	PartTypeSwitchButton            PartType = "Switch Button"
	PartTypeSparkArrester           PartType = "Spark arrester"
	PartTypeTransformer             PartType = "Transformer"
	PartTypeThermocouple            PartType = "Thermocouple"
	PartTypeTestPoint               PartType = "Test point"
	PartTypeTuner                   PartType = "Tuner"
	PartTypeIntegratedCircuit       PartType = "Integrated circuit (IC)"
	PartTypeVacuumTube              PartType = "Vacuum tube"
	PartTypeVoltageRegulator        PartType = "Voltage regulator"
	PartTypeSocketConnector         PartType = "Socket connector"
	PartTypeCrystalCeramicResonator PartType = "Crystal ceramic resonator powered oscillator"
	PartTypeZenerDiode              PartType = "Zener diode"
	PartTypeUnknown                 PartType = "Unknown/Unspecified Component"
)

var designatorTypes map[string]PartType = map[string]PartType{
	"A": PartTypeAssembly, "AT": PartTypeIsolator, "BR": PartTypeBridgeRectifier, "BAT": PartTypeBattery,
	"C": PartTypeCapacitor, "CB": PartTypeCircuitBreaker, "CN": PartTypeCapacitorNetwork,
	"D": PartTypeDiode, "DL": PartTypeDelayLine, "DN": PartTypeDiodeNetwork, "DS": PartTypeDisplay,
	"F": PartTypeFuse, "FB": PartTypeFerriteBead, "FD": PartTypeFiducial, "FL": PartTypeFilter,
	"G": PartTypeGeneratorOscillator, "GL": PartTypeGraphicalLogo, "GN": PartTypeGeneralNetwork,
	"H": PartTypeHardware, "HY": PartTypeCirculator,
	"J": PartTypeJackConnector, "JP": PartTypeJumper,
	"K": PartTypeRelayContactor,
	"L": PartTypeInductor, "LS": PartTypeLoudspeakerBuzzer,
	"M": PartTypeMotor, "MK": PartTypeMicrophone, "MP": PartTypeMechanicalPart,
	"OP": PartTypeOptoIsolator,
	"P":  PartTypePlugConnector, "PD": PartTypePhotodiode,
	"Q": PartTypeTransistor,
	"R": PartTypeResistor, "RN": PartTypeResistorNetwork, "RT": PartTypeThermistor, "RV": PartTypeVaristor,
	"S": PartTypeSwitchButton, "SA": PartTypeSparkArrester,
	"T": PartTypeTransformer, "TC": PartTypeThermocouple, "TP": PartTypeTestPoint, "TUN": PartTypeTuner,
	"U": PartTypeIntegratedCircuit,
	"V": PartTypeVacuumTube, "VR": PartTypeVoltageRegulator,
	"X":  PartTypeSocketConnector,
	"Y":  PartTypeCrystalCeramicResonator,
	"ZD": PartTypeZenerDiode,
}

var digitsPattern *regexp.Regexp = regexp.MustCompile(`\d+`)

// classifyDesignator maps a reference designator such as "C12" or "R3,R4" to its part type.
func classifyDesignator(designator string) PartType {
	upper := strings.ToUpper(strings.TrimSpace(designator))
	letters := digitsPattern.ReplaceAllString(upper, "")
	prefix, _, _ := strings.Cut(letters, ",")
	if t, ok := designatorTypes[prefix]; ok {
		return t
	}
	return PartTypeUnknown
}
