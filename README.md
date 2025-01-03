# log-tailor

A command line program for tailing GCP logs.

```bash
Usage of ./log-tailor:
  -f value
    	Filter expression (multiple ok)
  -format string
        Format: json,yaml,csv (default "yaml")
  -l value
    	Log to tail (short name, multiple ok)
  -limit int
    	Number of entries to output. Defaults to MaxInt
  -p value
    	Project ID (multiple ok)
```

Command line args override values in the config. You can pass a YAML config in via `stdin`. Here's a simple example for now:

```yaml
# all or drop-no-match (defaults to all)
match-rule: drop-no-match

projects:
- testing-proj

filters: []
# - protopayload.authenticationInfo.principalEmail = "system:gcp-controller-manager"

common-output:
- timestamp: timestamp
- project: resource.labels.project_id
- log-name:
    src: logname
    regex: projects/.*?/logs/(.*)
    value: $1
- resource-type: resource.type
- labels: resource.labels

logs:
- name: cloudaudit.googleapis.com/activity
  output:
  - payload: payload.protopayload
  - principalEmail: payload.protopayload.authenticationInfo.principalEmail
- name: cloudaudit.googleapis.com/data_access
  output:
  - payload: payload.protopayload
```

## Where it is now

You can specify logs, filters, projects, and output formats. If you want to customize (tailor) the output, you can specify a YAML config that maps values from the log entries to keys and values in the output.

## Where it may go

Right now it's project-based. There are logs at the billing-account and folder and organization level too. Support for those needs to be added.

