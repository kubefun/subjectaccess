# subjectaccess

List all of the resource access for a given kubernetes client.

## client config

The client passed to subjectaccess must be configured for high QPS and Burst.

    config.QPS = 50
    config.Burst = 250
