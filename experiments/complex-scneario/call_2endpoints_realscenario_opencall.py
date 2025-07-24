from locust import FastHttpUser, task, events, constant_pacing
import csv
import time
from datetime import datetime
import gevent

# CSV file setup
output_file = "out_realscenario_final.csv"

# Initialize and write the header for the CSV file
with open(output_file, mode="w", newline="") as file:
    writer = csv.writer(file)
    writer.writerow(["timestamp", "node", "endpoint", "response_time", "status_code", "response_body"])

def sanitize_response_body(response_body):
    """
    Replace newline characters in the response body with spaces or placeholders.
    """
    return response_body.replace('\n', ' ^^ ').replace('\r', '')

class SieveTestUser1(FastHttpUser):
    # Set constant_pacing to 1 second divided by desired RPS
    # For 2 RPS, use 0.5; for 10 RPS, use 0.1, etc.
    wait_time = constant_pacing(0.5)  # 2 requests per second
    host = "https://sieve-fx6refbs4a-oe.a.run.app"
         
    @task
    def sieve_request(self):
        start_time = time.time()
        timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        endpoint = ""

        payload = {}
        headers = {'Content-Type': 'application/json'}
        headers['Content-Length'] = "0" # custom header for gcp's 411 error code #str(len(json.dumps(payload)))
        def make_request():
            try:
                response = self.client.post(endpoint, headers=headers)
                response_time = (time.time() - start_time) * 1000  # Convert to ms
                status_code = response.status_code
                response_body = sanitize_response_body(response.text)
            except Exception as e:
                response_time = (time.time() - start_time) * 1000
                status_code = "N/A"
                response_body = sanitize_response_body(str(e))
                
            with open(output_file, mode="a", newline="") as file:
                writer = csv.writer(file)
                writer.writerow([timestamp, "GCP f", endpoint, response_time, status_code, response_body])
        
        # Spawn the request in a new greenlet
        gevent.spawn(make_request)

class SieveTestUser2(FastHttpUser):
    wait_time = constant_pacing(0.5)  # 2 requests per second
    host = "http://10.10.28.205:8000"
    
    @task
    def sieve_request(self):
        start_time = time.time()
        timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        endpoint = "/sieve"
        
        def make_request():
            try:
                response = self.client.post(endpoint)
                response_time = (time.time() - start_time) * 1000  # Convert to ms
                status_code = response.status_code
                response_body = sanitize_response_body(response.text)
            except Exception as e:
                response_time = (time.time() - start_time) * 1000
                status_code = "N/A"
                response_body = sanitize_response_body(str(e))
                
            with open(output_file, mode="a", newline="") as file:
                writer = csv.writer(file)
                writer.writerow([timestamp, "Raspi", endpoint, response_time, status_code, response_body])
        
        # Spawn the request in a new greenlet
        gevent.spawn(make_request)

# Global variable to store the test start time
test_start_time = None
environment_ref = None

@events.test_start.add_listener
def on_test_start(environment, **kwargs):
    global test_start_time, environment_ref
    test_start_time = time.time()
    environment_ref = environment
