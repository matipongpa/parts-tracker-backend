from re import match
import openpyxl
import csv
import sys
from enum import Enum

from part import Part, PartKey

csv_extension: str = '.csv'
data_started: bool = False
class Part_Type(Enum):
    UNKNOWN = 'Unknown'
    CAPACITOR = 'Capacitor'
    RESISTOR = 'Resistor'
    BATTERY = 'Battery'
    DIODE = 'Diode'
    INDUCTOR = 'Inductor'

def xlsx_save_csv_converter(xlsx_path: str) :
    try :
      # Load the Excel workbook and select the active sheet
        wb = openpyxl.load_workbook(xlsx_path)
        ws = wb.active

        file_name : str = xlsx_path.split('.')[0]
        print(file_name)

        csv_file_name: str = file_name + csv_extension

        with open(csv_file_name, mode='w', newline='') as file:
            writer = csv.writer(file)
            for row in ws.iter_rows(values_only=True):
                writer.writerow(row)
        with open(csv_file_name,mode='r') as file :
            csv_reader = csv.reader(file)
            # Iterate over each row in the CSV file
            for row in csv_reader:
                update_bom(row)

    except FileNotFoundError:
        print(f"Error: Input file '{xlsx_path}' not found.")
    except Exception as e:
        print(f"An error occurred: {e}")

def update_bom(csv_row : list[str]) :
    global data_started
    part: Part = {
        "type": '',
        "fit": '',
        "footprint": '',
        "manufacturer": '',
        "part_number": '',
        "value": ''
    }
    if data_started and not'Designator' in csv_row:
        #csv_row[1] = description it provide type of the device
        description = csv_row[1]
        part_type_found : Part_Type = Part_Type.UNKNOWN
      
        if Part_Type.CAPACITOR.value.lower() in description.lower():
            part_type_found = Part_Type.CAPACITOR
        elif Part_Type.RESISTOR.value.lower() in description.lower():
            part_type_found = Part_Type.RESISTOR
        elif Part_Type.BATTERY.value.lower() in description.lower():
            part_type_found = Part_Type.BATTERY
        elif Part_Type.DIODE.value.lower() in description.lower():
            part_type_found = Part_Type.DIODE
        elif Part_Type.INDUCTOR.value.lower() in description.lower():
            part_type_found = Part_Type.INDUCTOR
        else:
            print(f"Type unknown for description: '{description}'. Skipping.")
            return

        print(part_type_found.value)
        
        if part_type_found != Part_Type.UNKNOWN:
            part[PartKey.type.value] = part_type_found.value.lower()
            print(part)

    if not data_started and 'Designator' in csv_row:
        # Assuming 'Designator' is the first element of the actual header row
        if csv_row[0] == 'Designator':
            header = csv_row
            data_started = True

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python converter.py {file name}.xlsx")
    else:
       xlsx_save_csv_converter(sys.argv[1])