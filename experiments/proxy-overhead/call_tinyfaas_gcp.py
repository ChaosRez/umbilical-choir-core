from locust import FastHttpUser, task, events, constant_pacing
import csv
import time
from datetime import datetime
import gevent

# CSV file setup
output_file = "out_tinyfaas_gcp.csv"

# Initialize and write the header for the CSV file
with open(output_file, mode="w", newline="") as file:
    writer = csv.writer(file)
    writer.writerow(["timestamp", "user", "endpoint", "response_time", "status_code", "response_body"])

def sanitize_response_body(response_body):
    """
    Replace newline characters in the response body with spaces or placeholders.
    """
    return response_body.replace('\n', ' ^^ ').replace('\r', '')

class LoadTestUser(FastHttpUser):
    # Define the request pacing (e.g., 2 requests per second)
    wait_time = constant_pacing(0.5)  # Adjust for desired RPS
    host = "http://34.32.25.186:8000"

    @task
    def load_test_request(self):
        start_time = time.time()
        timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        endpoint = "/sieve"

        payload = {}
        headers = {'Content-Type': 'application/json', 'X-Function-Choice': 'f2'}
        headers['Content-Length'] = "0" # custom header for gcp's 411 error code #str(len(json.dumps(payload)))

        def make_request():
            try:
                response = self.client.post(endpoint, headers=headers)
                response_time = (time.time() - start_time) * 1000  # Convert to milliseconds
                status_code = response.status_code
                response_body = sanitize_response_body(response.text)
            except Exception as e:
                response_time = (time.time() - start_time) * 1000
                status_code = "N/A"
                response_body = sanitize_response_body(str(e))

            # Log data to CSV
            with open(output_file, mode="a", newline="") as file:
                writer = csv.writer(file)
                writer.writerow([timestamp, "LoadTestUser", endpoint, response_time, status_code, response_body])

        # Use gevent to spawn the request asynchronously
        gevent.spawn(make_request)

# Global variables to store test state
test_start_time = None
environment_ref = None

@events.test_start.add_listener
def on_test_start(environment, **kwargs):
    """
    Initialize test start time and environment on test start.
    """
    global test_start_time, environment_ref
    test_start_time = time.time()
    environment_ref = environment

@events.test_stop.add_listener
def on_test_stop(environment, **kwargs):
    """
    Log a message or perform cleanup at the end of the test.
    """
    elapsed_time = time.time() - test_start_time
    print(f"Test completed. Total runtime: {elapsed_time:.2f} seconds")

