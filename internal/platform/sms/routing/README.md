# SMS routing

The default priority routes are:

- Ghana: `mnotify` (priority 1), then `moolre` (priority 2)
- Nigeria: `leamout` (priority 1), then `runnage` (priority 2)

The routing strategy permits fallback only when a provider error explicitly
reports that the request was definitively rejected before acceptance. Unknown
or uncertain outcomes stop routing to avoid duplicate SMS delivery.
