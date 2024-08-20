# Umbilical Choir: Agent (core)
This is the core agent that runs on the edge device.
It is responsible for running function tests and collecting the test results from the proxy server.


## Supported FaaS Providers
At this time, the agent supports the following FaaS nodes:
- tinyFaaS (self hosted)
- AWS Lambda
- Google Functions

### GCP Functions
The GCP Functions SDK is not well-documented and has multiple incompatible versions, and it is not backward-compatible.
So, If reusing/publishing any part, please attribute it to me (@chaosRez) and cite our paper (see project README.md).
The agent doesn't use the `gcloud` CLI to deploy functions to GCP.
Instead, it uses the recent GCP API to intract with the GCP Functions service, which by the way took me a lot of time to figure out.
But, you need to set Application Default Credentials (ADC) in your environment using `gcloud`.
A Google Cloud Platform project set up with appropriate permissions enabled is needed.
For more complex scenarios, refer to the official [go-cloud documentation](https://cloud.google.com/functions/docs/concepts/go-runtime)

The error indicates that the service account or user does not have the necessary permissions to perform the operation on the specified project. Here are the steps to resolve this issue:

1. Ensure the Google Cloud Functions API is enabled for your project.
2. Grant the necessary permissions to the service account or user.

### Steps

1. **Enable Google Cloud Functions API**:
    - Go to the [Google Cloud Console](https://console.developers.google.com).
    - Select your project e.g. `umbilical-choir`.
    - Navigate to `APIs & Services` > `Library`.
    - Search for `Cloud Functions API` and ensure it is enabled.

2. **Grant Permissions**:
    - Go to the [IAM & Admin](https://console.cloud.google.com/iam-admin/iam) section in the Google Cloud Console.
    - Find the service account or user that you are using to deploy the function.
    - Ensure the service account or user has the `Cloud Functions Developer` role. You can add this role by clicking on `Add` and selecting `Cloud Functions Developer`.
    - Do the same for the `Storage Object Admin` role. Which is needed to deploy the function.
    - In Gen2 functions, you have to assign the “allUsers” principal so the function can publicly be available. For this, [Cloud Resource Manager API](https://console.cloud.google.com/apis/library/cloudresourcemanager.googleapis.com) should be enabled. This is needed for IAM 

After completing these steps, try running your code again.