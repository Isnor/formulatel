# formulatel protocol buffers

This folder contains the protocol buffers used by formulatel


## Goals

* Standard, title-agnostic way of representing vehicle telemetry.
* Represent metrics as open-telemetry metrics
* Chart telemetry in a title-agnostic way

The hope is eventually we'll be able to chart things in a consistent way, regardless of the title providing the telemetry. I don't have access to enough telemetry specs to know if that's a reasonable goal - it could very well turn out that not every game has the same concept of every sensor that the vehicle has, but it's still fun to try.