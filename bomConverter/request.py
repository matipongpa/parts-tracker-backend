import requests
import json

class ClientService:
    def __init__(self, base_url: str):
        if not base_url.endswith('/'):
            base_url += '/' # Ensure base_url ends with a slash for consistent path joining
        self.base_url = base_url
        self.session = requests.Session() # Use a session for persistent connections and cookies

    def _make_request(self, method: str, endpoint: str, **kwargs):
        url = f"{self.base_url}{endpoint.lstrip('/')}" # Ensure endpoint doesn't start with / if base_url has it
        print(f"\n--- Sending {method.upper()} Request to {url} ---")
        try:
            self.session.headers = kwargs['headers']
          
            response = self.session.request(method, url, json=kwargs['json'])
            response.raise_for_status() # Raise an HTTPError for bad responses (4xx or 5xx)
            print("Status Code:", response.status_code)
            
            # Try to parse JSON if Content-Type indicates it
            if 'application/json' in response.headers.get('Content-Type', ''):
                print("Response JSON:", response.json())
            else:
                print("Response Body:", response.text)
            return response
        except requests.exceptions.HTTPError as e:
            print(f"HTTP Error: {e.response.status_code} - {e.response.text}")
            return None
        except requests.exceptions.ConnectionError as e:
            print(f"Connection Error: Could not connect to {url}. Is the Go server running?")
            print(f"Details: {e}")
            return None
        except requests.exceptions.Timeout as e:
            print(f"Timeout Error: Request to {url} timed out. Details: {e}")
            return None
        except requests.exceptions.RequestException as e:
            print(f"An unexpected request error occurred: {e}")
            return None

    def post_part_json(self, data: dict):
        headers = {"Content-Type": "application/json"}
        return self._make_request("POST", "/part", json=data, headers=headers)
