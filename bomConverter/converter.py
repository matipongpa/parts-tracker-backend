from re import match
import csv
import sys
from enum import Enum

from csvHandler import CsvHandler
from request import ClientService
from part import Part, PartKey, PartType

class App():
    def __init__(self,excelFileName: str, severUrl:str):
        self.urlRequest: str =  severUrl
        self.excelFile : str = excelFileName
        self.client: ClientService = None
        self.csvHandler: CsvHandler = None
        self.partList: list[Part] = []
        if (self.client == None) :
            self.client = ClientService(self.urlRequest)
        if (self.csvHandler == None) :
            self.csvHandler = CsvHandler(self.excelFile)  
    def Run(self) :
        self.partList = self.csvHandler.xlsx_save_csv_converter()
        if self.partList != [] :
            for part in self.partList:
                response = self.client.post_part_json(part)
                print("response",response)
        
if __name__ == "__main__":
    if len(sys.argv) != 3 :
        print("Usage: python converter.py {file name}.xlsx {serverurl}")
    else:
      app = App(sys.argv[1],sys.argv[2])
      app.Run()
