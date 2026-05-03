1) A single ingest instance will only be receiving telemetry from a single title at once, so I guess we'll probably need to add a flag/command to the `ingest` CLI to tell it which title it's going to receive telemetry for so that it can start the correct transformer

2) We are currently only ingesting a single type of data from f123: "VehicleData". Let's finish the conversion by unpacking the array of tire data to our flattened format. Then we'll begin adding motion data and other types of packets from f123.


99) The reason is that Grafana Live only supports v3. I'd love to contribute an extension / new plugin to Grafana that uses the v5 API, but I don't know how to do that.