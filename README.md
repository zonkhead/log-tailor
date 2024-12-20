# log-tailor

A command line program for tailing GCP logs.

```bash
Usage of ./log-tailor:
  -f value
    	Filter expression (multiple ok)
  -format string
    	Format: json,yaml (default "yaml")
  -l value
    	Log to tail (short name, multiple ok)
  -limit int
    	Number of entries to output. Defaults to 0 which is no-limit
  -p string
    	Project ID
```

## Where it is now

Very simple. Lets you specify a projectID and log names and filters.

## Where it may go

Not sure yet. Originally, I was thinking allowing manipulation of the output but `jq` and `yq` do well at that.
