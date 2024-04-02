# f123 protocol buffers

This folder contains protocol buffers specific to the F123 title and map directly to the packet [specification](https://answers.ea.com/t5/General-Discussion/F1-23-UDP-Specification/m-p/12633159?attachment-id=704910).

We may not _need_ these, but because we aren't currently able to port-forward UDP and the telemetry data only comes via UDP, something (currently `ingest`) needs to send that data from the UDP listener to `formulatel`. I figured the best way to do that is probably just using gRPC because `ingest` is already unpacking every packet it receives and sending it along to the backend

- unless you want ingest to do the conversion from title<->formulatel, you'll need to define a `GeneralMessage` type that holds the raw packet payload for `formulatel` to be able to route it appropriately. something like:

```proto
message TelemetryData {
  GameTitle Title = 1;
  // how would we, from ingest or otherwise, create a session ID? could/should we use an IP address? UDP doesn't really have a "connection",
  // so maybe for now we'll rely on the title to give us a session ID.
  string SessionID = 2; // a unique ID for every session of telemetry; this should correspond to the single telemetry source
  // TODO: what if we actually have many telemetry sources for one session? e.g. an online session where more than 1 player sends telemetry
  // to a formulatel backend
  string UserID = 3; // identify the user within the session; each (sessionID, userID) is a unique vehicle 
  bytes Data = 4;
}
```