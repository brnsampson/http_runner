# http_runner
Functions that accept any http.Handler and implement boilerplate needed for sane server management

## What ##

This is just a way for me to prevent too much boilerplate from leaking into all of my projects. Doing the actual http
server construction and signal handling is generally pretty monotonous; the interesting part is in the server mux and
tls configuration.

Of course, this always uses the standard http.Server. On the other hand, you can use a variety of routers and muxes with
the http.Server so that's not that big of a downside for me. Chi is my router of choice, but gorilla/mux and a variety of
other ones work perfectly well.
