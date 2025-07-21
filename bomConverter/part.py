from enum import Enum
from typing import TypedDict


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