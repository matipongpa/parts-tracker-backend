from re import match
import openpyxl
import csv
from part import Part, PartKey, PartType


class CsvHandler:
    def __init__(self, excelFileName: str):
        self.excelFile = excelFileName
        self.csv_extension: str = '.csv'
        self.data_started: bool = False
        self.projectNameKey : str = 'Project Name :'
        self.projectCodeKey : str = 'Project Code :'
        self.projectName: str = ''
        self.projectCode :str = ''
        self.partList: list[Part] = []
    
    def xlsx_save_csv_converter(self) -> 'list[Part]':
        try :
        # Load the Excel workbook and select the active sheet
            excelFileName = self.excelFile
            wb = openpyxl.load_workbook(excelFileName)
            ws = wb.active

            file_name : str = excelFileName.split('.')[0]

            csv_file_name: str = file_name + self.csv_extension

            with open(csv_file_name, mode='w', newline='') as file:
                writer = csv.writer(file)
                for row in ws.iter_rows(values_only=True):
                    writer.writerow(row)
            with open(csv_file_name,mode='r') as file :
                csv_reader = csv.reader(file)
                # Iterate over each row in the CSV file
                for row in csv_reader:
                    if self.projectName == '' or self.projectCode == '' : 
                        if self.projectNameKey in row :
                            self.projectName = row[1]
                        elif self.projectCodeKey in row :
                            self.projectCode = row[1]
                    else :
                        self.update_bom(row)
            return self.partList
        except FileNotFoundError:
            print(f"Error: Input file '{excelFileName}' not found.")
            return self.partList
        except Exception as e:
            print(f"An error occurred: {e}")
            return self.partList


    def update_bom(self,csv_row : list[str]) :
        part: Part = {
            "type": '',
            "fit": '',
            "footprint": '',
            "manufacturer": '',
            "part_number": '',
            "value": '',
            "project_name": '',
            "project_code": '',
            "description": ''
        }
        if self.data_started and not'Designator' in csv_row:
            #csv_row[1] = description it provide type of the device
            designator = csv_row[0]
            if designator == '' :
                return
            part_type_found  = PartType.get_part_type(designator)
                
            if part_type_found != PartType.UNKNOWN:
                part[PartKey.type.value] = part_type_found.value.lower()
                part[PartKey.description.value] = PartType.clean_text(csv_row[1])
                part[PartKey.fit.value] = csv_row[2]
                part[PartKey.footprint.value] = PartType.clean_text(csv_row[3])
                part[PartKey.manufacturer.value] = PartType.clean_text(csv_row[4])
                part[PartKey.part_number.value] = csv_row[5]
                part[PartKey.value.value] = PartType.clean_text(csv_row[7])
                part[PartKey.project_name.value] = self.projectName
                part[PartKey.project_code.value] = self.projectCode
                print("part before send")
                self.partList.append(part)
        if not self.data_started and 'Designator' in csv_row:
            # Assuming 'Designator' is the first element of the actual header row
            if csv_row[0] == 'Designator':
                self.data_started = True
