# subjectaccess

List all of the resource access for a given kubernetes client.

## client config

The client passed to subjectaccess must be configured for high QPS and Burst.

    config.QPS = 500
    config.Burst = 1000
