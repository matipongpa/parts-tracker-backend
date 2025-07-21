from enum import Enum
from typing import TypedDict
import re


class Part(TypedDict):
    type: str
    fit: str
    footprint: str
    manufacturer: str
    part_number: str
    value: str

class PartKey(Enum):
    type = 'type'
    fit = 'fit'
    footprint = 'footprint'
    manufacturer = 'manufacturer'
    part_number = 'part_number'
    value = 'value'
class PartType(Enum) : 
    A = "assembly"
    AT = "isolator"
    BR = "Bridge-rectifier"
    BAT = "Battery"
    C = "Capacitor"
    CB = "Circuit breaker"
    CN = "Capacitor network"
    D = "Diode"
    DL = "Delay line"
    DN = "Diode network"
    DS = "Display"
    F = "Fuse"
    FB = "Ferrite bead"
    FD = "Fiducial"
    FL = "Filter"
    G = "Generator/Oscillator"
    GL = "Graphical logo"
    GN = "General network"
    H = "Hardware"
    HY = "Circulator"
    J = "Jack connector"
    JP = "Jumper"
    K = "Relay contactor"
    L = "Inductor"
    LS = "Loudspeaker buzzer"
    M = "Motor"
    MK = "Microphone"
    MP = "Mechanical part"
    OP = "Opto-isolator"
    P = "Plug connector"
    PD = "Photodiode"
    Q = "Transistor"
    R = "Resistor"
    RN = "Resistor network"
    RT = "Thermistor"
    RV = "Varistor"
    S = "Switch Button"
    SA = "Spark arrester"
    T = "Transformer"
    TC = "Thermocouple"
    TP = "Test point"
    TUN = "Tuner"
    U = "Integrated circuit (IC)"
    V = "Vacuum tube"
    VR = "Voltage regulator"
    X = "Socket connector"
    Y = "Crystal ceramic resonator powered oscillator"
    ZD = "Zener diode"
    UNKNOWN = "Unknown/Unspecified Component"

    @classmethod
    def get_part_type(cls,designation: str) -> 'PartType':
        delimiter : str = ','

        designation__synbol_upper: str = designation.strip().upper()
        designation_no_num: str = re.sub(r'\d+', '',designation__synbol_upper)


        for part_type in PartType:
            if part_type.name.lower() in designation.lower():
                return part_type
        return PartType.UNKNOWN