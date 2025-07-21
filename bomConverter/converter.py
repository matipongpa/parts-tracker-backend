import openpyxl
import csv
import sys

csv_extension: str = '.csv'

def xlsx_save_csv_converter(xlsx_path: str) :
    print(xlsx_path)
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
            print(row)

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python converter.py {file name}.xlsx")
    else:
       xlsx_save_csv_converter(sys.argv[1])