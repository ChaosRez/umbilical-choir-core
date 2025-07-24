# Experiment Scripts

This folder contains scripts for running Locust-based experiments from the paper.
A more proper testing is TODO.  
The initial client was `k8`, but it has been replaced with `locust` for better flexibility with open-system calls.

## Proxy Overhead
```sh
locust -f call_raspberry.py
locust -f call_gcp.py
locust -f call_tinyfaas_gcp.py
```

## Complex Scenario
```sh
locust -f call_2endpoints_realscenario_opencall.py -u 2 -r 1
```

