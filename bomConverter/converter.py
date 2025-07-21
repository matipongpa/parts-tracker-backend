from re import match
import openpyxl
import csv
import sys
from enum import Enum

from part import Part, PartKey, PartType

csv_extension: str = '.csv'
data_started: bool = False
i = 1
projectNameKey = 'Project Name :'
projectCodeKey = 'Project Code :'

def xlsx_save_csv_converter(xlsx_path: str) :
    global projectNameKey
    global projectCodeKey

    projectCode = ''
    projectName = ''
    try :
      # Load the Excel workbook and select the active sheet
        wb = openpyxl.load_workbook(xlsx_path)
        ws = wb.active

        file_name : str = xlsx_path.split('.')[0]

        csv_file_name: str = file_name + csv_extension

        with open(csv_file_name, mode='w', newline='') as file:
            writer = csv.writer(file)
            for row in ws.iter_rows(values_only=True):
                writer.writerow(row)
        with open(csv_file_name,mode='r') as file :
            csv_reader = csv.reader(file)
            # Iterate over each row in the CSV file
            for row in csv_reader:
                if projectName == '' or projectCode == '' : 
                    if projectNameKey in row :
                        projectName = row[1]
                    elif projectCodeKey in row :
                        projectCode = row[1]
                else :
                    update_bom(row,projectName,projectCode)

    except FileNotFoundError:
        print(f"Error: Input file '{xlsx_path}' not found.")
    except Exception as e:
        print(f"An error occurred: {e}")


def update_bom(csv_row : list[str],projectName:str, projectCode:str) :
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
        designator = csv_row[0]
        if designator == '' :
            return
        part_type_found  = PartType.get_part_type(designator)
               
        if part_type_found != PartType.UNKNOWN:
            part[PartKey.type.value] = part_type_found.value.lower()
            part[PartKey.fit.value] = csv_row[2]
            part[PartKey.footprint.value] = csv_row[3]
            part[PartKey.manufacturer.value] = csv_row[4]
            part[PartKey.part_number.value] = csv_row[5]
            part[PartKey.value.value] = csv_row[7]
            part[PartKey.project_name.value] = projectName
            part[PartKey.project_code.value] = projectCode
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