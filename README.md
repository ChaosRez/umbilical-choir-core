# Umbilical Choir: Agent (core)
This is the core agent that runs on the edge device.
It is responsible for running function tests and collecting the test results from the proxy server.

## Writing release strategies
The release strategy is defined in a human-readable YAML format.
### stage's "end_action"
The `end_action` of a stage can be one of the following on `onSuccess` and `onFailure` keys:
```yaml
onSuccess: rollout # or rollback, or a specific (next) stage 
onFailure: rollback
```

## Function Format
For nodejs functions, the agent expects an "index.js" file where the main function is defined in a outer `moudle`/`exports` format.
For python functions, the agent expects a "fn.py" file where the main function is defined in a outer `def fn(input: typing.Optional[str], headers: typing.Optional[typing.Dict[str, str]]) -> typing.Optional[str]:` format (tinyFaaS standard format).

## Supported FaaS Providers
At this time, the agent supports the following FaaS nodes and Runtimes:
- tinyFaaS (self hosted)
  - nodejs, and python3
  - Assumes tinyFaaS is running on the same machine as the agent
- AWS Lambda
  - ???
- Google Functions
  - nodejs20, python312

### GCP Functions
The GCP Functions SDK is not well-documented and has multiple incompatible versions, and it is not backward-compatible.
So, If reusing/publishing any part, please attribute it to me (@chaosRez) and cite our paper (see project README.md).
The agent doesn't use the `gcloud` CLI to deploy functions to GCP.
Instead, it uses the recent GCP API to interact with the GCP Functions service, which by the way took me a lot of time to figure out.
But, you need to set Application Default Credentials (ADC) in your environment using `gcloud`.
A Google Cloud Platform project set up with appropriate permissions enabled is needed.
For more complex scenarios, refer to the official [go-cloud documentation](https://cloud.google.com/functions/docs/concepts/go-runtime)

The function source priority is as follows:  
1. SourceZipURL
2. SourceLocalPath
3. SourceGitRepoURL

Pre-requisites:

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
