# TODO

## General

* only works on second run? (first run acts
* off by .2MHz sometimes?
* hangs if I2C is wedged (ex. CTRL-C doesn't work)
* is pretty fragile to I2C state, doesn't accurately detect I2C state if it's not quite right


- - - -

## Code Cleanup

* `gofm.go` is littered with comments, badly structured code, etc.
* fill out constants for Si4703
* remove tcell dependency
* previous work (202005) left off with `rds.go:update_pin()`
* accept frequency as first argument
* `--scan` to scan the FM band for stations on startup
* `--stream` to skip the UI and instead stream RDS data to stdout

- - - -

## UI

* frequency background should be actual black
* whitespace trim program identifier
* scroll the large program identifier (and/or smaller font)
* clear all fields when changing channel
* dyn prog type, alt freqs
* (S)can band: stations, strengths, stereo, traffic, program type
* show RDS codes 0A..15B, decay to dark gray, one step per sec?
* left/right arrows change channel, up/down arrows change volume
* freq could animate, with fading to the edges to resemble a dial

- - - -

## RDS decoding

* left off at: update_pin()
* add database for 3-char stations
* verify comments
* much of this state should be scoped to the Program Information fields
    * this would allow "band memory" of what stations are where, strengths, program types, stereo, RDS channels, etc.
* low-hanging fruit group types
    * 4A: Clock-time and date (every minute)
    * 8A: Traffic Message Channel (CEN standard ENV 12313-1)
    * 12A/B: Open Data ... but can I reverse engineer it?
* investigate
    * 5A/B: Transparent Data Channels ("alphanumeric characters, or other text (including mosaic graphics), or computer programs and similar
 data not for display.")
    * 6A/B: In-house applications
    * 7A: Radio Paging
    * 9A: Emergency warning systems
    * 10A: Program Type Name (more PT info)
    * 13A: Enhanced Radio Paging
    * 14A/B: Enhanced Other Networks
    * 15B: Fast basic tuning and switching
* traffic?  Alert-C

