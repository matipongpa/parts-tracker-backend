from enum import Enum
from typing import TypedDict
import re


class Part(TypedDict):
    type: str
    description:str
    fit: str
    footprint: str
    manufacturer: str
    part_number: str
    value: str
    project_name: str
    project_code: str

class PartKey(Enum):
    type = 'type'
    description = 'description'
    fit = 'fit'
    footprint = 'footprint'
    manufacturer = 'manufacturer'
    part_number = 'part_number'
    value = 'value'
    project_name = 'project_name'
    project_code = 'project_code'
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
        designator_final: str = ''
        designation__synbol_upper: str = designation.strip().upper()
        designation_no_num: str = re.sub(r'\d+', '',designation__synbol_upper)
        if delimiter in designation_no_num :
            designator_final = designation_no_num.split(',')[0]        
        else :
            designator_final =  designation_no_num
        for part_type in PartType:
            if designator_final == part_type.name:
                return part_type
        return PartType.UNKNOWN