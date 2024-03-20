# Formula Telemetry

I like playing the F1 simarcade games and I like OpenTelemetry. I thought it would be a fairly painless lift to read the telemetry data because the spec is publicly available and chart it with OpenTelemetry and Grafana. Then I thought it might be helpful to convert the data into protocol buffers so that others could easily make their own telemetry applications.

Some more fun things I want to do with it:
- gRPC service to ingest the data using protobufs. not sure if this would be very useful for the realtime charting, but it could be for storing tables of data if somebody wanted to make a webservice
- kafka streams?

## TODO:

I'm too easily distracted

I don't know if we need to separate/move modules into separate dirs, but this is getting annoying. It shouldn't be difficult to separate the front and backend(s) for this and easily write them individually, so what's wrong?

The RPC service is probably the easiest to write the implementations for because it just requires the handlers to put the data they receive into a data store/queue/etc.

The frontend is causing me confusion because it is also a server, but it's a client implementation of the RPC service. The data it receives and the way it translates it to the protobufs is dependent on the particular telemetry system that is producing it.