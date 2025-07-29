package part

import (
	"regexp"
	"strings"
)

// Part represents the structure of a single part.
type Part struct {
	Type         string `json:"type"`
	Description  string `json:"description"`
	Fit          string `json:"fit"`
	Footprint    string `json:"footprint"`
	Manufacturer string `json:"manufacturer"`
	PartNumber   string `json:"part_number"`
	Value        string `json:"value"`
	ProjectName  string `json:"project_name"`
	ProjectCode  string `json:"project_code"`
}

// PartKey defines the keys for a Part. (Less critical in Go, but good for consistency)
type PartKey string

const (
	PartKeyType         PartKey = "type"
	PartKeyDescription  PartKey = "description"
	PartKeyFit          PartKey = "fit"
	PartKeyFootprint    PartKey = "footprint"
	PartKeyManufacturer PartKey = "manufacturer"
	PartKeyPartNumber   PartKey = "part_number"
	PartKeyValue        PartKey = "value"
	PartKeyProjectName  PartKey = "project_name"
	PartKeyProjectCode  PartKey = "project_code"
)

// HeaderIndex maps header names to their corresponding string values.
type HeaderIndex string

const (
	HeaderIndexType         HeaderIndex = "Type"
	HeaderIndexDescription  HeaderIndex = "Description"
	HeaderIndexFit          HeaderIndex = "FIT/NFIT"
	HeaderIndexFootprint    HeaderIndex = "Footprint"
	HeaderIndexManufacturer HeaderIndex = "Manufacturer"
	HeaderIndexPartNumber   HeaderIndex = "Manufacturer part no."
	HeaderIndexValue        HeaderIndex = "Value"
	HeaderIndexProjectName  HeaderIndex = "project_name"
	HeaderIndexProjectCode  HeaderIndex = "project_code"
)

// PartType represents the type of a component.
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

// GetPartType determines the PartType based on the given designation string.
func GetPartType(designation string) PartType {
	delimiter := ","
	designationSymbolUpper := strings.ToUpper(strings.TrimSpace(designation))
	re := regexp.MustCompile(`\d+`)
	designationNoNum := re.ReplaceAllString(designationSymbolUpper, "")

	designatorFinal := ""
	if strings.Contains(designationNoNum, delimiter) {
		designatorFinal = strings.Split(designationNoNum, delimiter)[0]
	} else {
		designatorFinal = designationNoNum
	}

	switch designatorFinal {
	case "A":
		return PartTypeAssembly
	case "AT":
		return PartTypeIsolator
	case "BR":
		return PartTypeBridgeRectifier
	case "BAT":
		return PartTypeBattery
	case "C":
		return PartTypeCapacitor
	case "CB":
		return PartTypeCircuitBreaker
	case "CN":
		return PartTypeCapacitorNetwork
	case "D":
		return PartTypeDiode
	case "DL":
		return PartTypeDelayLine
	case "DN":
		return PartTypeDiodeNetwork
	case "DS":
		return PartTypeDisplay
	case "F":
		return PartTypeFuse
	case "FB":
		return PartTypeFerriteBead
	case "FD":
		return PartTypeFiducial
	case "FL":
		return PartTypeFilter
	case "G":
		return PartTypeGeneratorOscillator
	case "GL":
		return PartTypeGraphicalLogo
	case "GN":
		return PartTypeGeneralNetwork
	case "H":
		return PartTypeHardware
	case "HY":
		return PartTypeCirculator
	case "J":
		return PartTypeJackConnector
	case "JP":
		return PartTypeJumper
	case "K":
		return PartTypeRelayContactor
	case "L":
		return PartTypeInductor
	case "LS":
		return PartTypeLoudspeakerBuzzer
	case "M":
		return PartTypeMotor
	case "MK":
		return PartTypeMicrophone
	case "MP":
		return PartTypeMechanicalPart
	case "OP":
		return PartTypeOptoIsolator
	case "P":
		return PartTypePlugConnector
	case "PD":
		return PartTypePhotodiode
	case "Q":
		return PartTypeTransistor
	case "R":
		return PartTypeResistor
	case "RN":
		return PartTypeResistorNetwork
	case "RT":
		return PartTypeThermistor
	case "RV":
		return PartTypeVaristor
	case "S":
		return PartTypeSwitchButton
	case "SA":
		return PartTypeSparkArrester
	case "T":
		return PartTypeTransformer
	case "TC":
		return PartTypeThermocouple
	case "TP":
		return PartTypeTestPoint
	case "TUN":
		return PartTypeTuner
	case "U":
		return PartTypeIntegratedCircuit
	case "V":
		return PartTypeVacuumTube
	case "VR":
		return PartTypeVoltageRegulator
	case "X":
		return PartTypeSocketConnector
	case "Y":
		return PartTypeCrystalCeramicResonator
	case "ZD":
		return PartTypeZenerDiode
	default:
		return PartTypeUnknown
	}
}
