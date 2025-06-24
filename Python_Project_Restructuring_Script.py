import os
import shutil

def create_directory(path):
    if not os.path.exists(path):
        os.makedirs(path)

def move_file(src, dest):
    shutil.move(src, dest)

def create_file(path, content=''):
    with open(path, 'w') as f:
        f.write(content)

def restructure_project():
    # Create new directory structure
    create_directory('link_budget_calculator')
    create_directory('link_budget_calculator/satellite')
    create_directory('link_budget_calculator/antenna')
    create_directory('link_budget_calculator/receiver')
    create_directory('link_budget_calculator/link')
    create_directory('tests')

    # Move and rename files
    move_file('link-budget-demo.py', 'link_budget_calculator/calculator.py')
    move_file('input_data.py', 'link_budget_calculator/input_data.py')
    move_file('test_link_budget_demo.py', 'tests/test_calculator.py')

    # Split classes into separate files
    with open('link_budget_calculator/input_data.py', 'r') as f:
        content = f.read()

    # Create new files for each class
    class_files = {
        'SatelliteInputs': 'link_budget_calculator/satellite/inputs.py',
        'AntennaInputs': 'link_budget_calculator/antenna/inputs.py',
        'ReceiverStationInputs': 'link_budget_calculator/receiver/inputs.py',
        'LinkParameters': 'link_budget_calculator/link/parameters.py',
    }

    for class_name, file_path in class_files.items():
        start = content.index(f'class {class_name}')
        end = content.index('class', start + 1) if 'class' in content[start + 1:] else None
        class_content = content[start:end].strip()
        create_file(file_path, class_content)

    # Update InputData class
    input_data_content = """from .satellite.inputs import SatelliteInputs
from .antenna.inputs import AntennaInputs
from .receiver.inputs import ReceiverStationInputs
from .link.parameters import LinkParameters

class InputData:
    def __init__(self, satellite, antenna, receiver_station, link_params):
        self.satellite = satellite
        self.antenna = antenna
        self.receiver_station = receiver_station
        self.link_params = link_params
"""
    create_file('link_budget_calculator/input_data.py', input_data_content)

    # Create __init__.py files
    init_files = [
        'link_budget_calculator/__init__.py',
        'link_budget_calculator/satellite/__init__.py',
        'link_budget_calculator/antenna/__init__.py',
        'link_budget_calculator/receiver/__init__.py',
        'link_budget_calculator/link/__init__.py',
    ]
    for file in init_files:
        create_file(file)

    # Update imports in calculator.py
    with open('link_budget_calculator/calculator.py', 'r') as f:
        calculator_content = f.read()
    
    updated_imports = """from .input_data import InputData
from .satellite.inputs import SatelliteInputs
from .antenna.inputs import AntennaInputs
from .receiver.inputs import ReceiverStationInputs
from .link.parameters import LinkParameters
"""
    updated_calculator_content = updated_imports + calculator_content[calculator_content.index('class Satellite'):]
    create_file('link_budget_calculator/calculator.py', updated_calculator_content)

    # Update imports in test_calculator.py
    with open('tests/test_calculator.py', 'r') as f:
        test_content = f.read()
    
    updated_test_imports = """import unittest
from link_budget_calculator.calculator import LinkBudgetCalculator
from link_budget_calculator.input_data import InputData
from link_budget_calculator.satellite.inputs import SatelliteInputs
from link_budget_calculator.antenna.inputs import AntennaInputs
from link_budget_calculator.receiver.inputs import ReceiverStationInputs
from link_budget_calculator.link.parameters import LinkParameters
"""
    updated_test_content = updated_test_imports + test_content[test_content.index('class TestLinkBudgetCalculator'):]
    create_file('tests/test_calculator.py', updated_test_content)

    print("Project restructuring completed successfully!")

if __name__ == "__main__":
    restructure_project()