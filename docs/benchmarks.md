# Benchmarks: conform against models from the field

`spine conform` tried against four state machines that neither the toolkit's authors nor its
examples wrote: models learned from a TLS server, an MQTT broker and a bank card, and a textbook
coffee machine, each with faults planted in it mechanically (#106). The drawings show the four
models as published.

Measured on 2026-11-08 with `spine` as of `6e3b166`, on 16 cores:
`go -C tools test ./internal/benchmarks/ -full -v -timeout 1h`. The mutants are chosen with a
fixed seed, so every number but the times comes out the same on every run. Plain
`go -C tools test ./...` runs one mutant of each class, model and translation, in about 10 s.

## What is tested

ADR 0008 says that with `observe()` reading the state, conform, from a shortest path to each
state plus every sequence of 1 to K+1 further steps, catches every deterministic implementation
that has at most K more states than the spine (Vasilevskii 1973, Chow 1978). The benchmark plants
faults where the answer is known in advance and checks conform's verdict on each:

- **Two translations.** Each model, a Mealy machine whose every step gives an output and a next
  state, becomes a spine twice: once with its outputs dropped, since a spine has none, and once
  under the Mealy-to-Moore fold, which names each state with the output it was entered by, so that
  outputs become states.
- **The code** is the model, or a mutant of it, run by an adapter the test binary serves.
  `observe()` reports the state's name, and under the fold the output it was entered by too.
- **Four classes of mutant,** the field's standard ones: every cell sent to every other state
  (transfer faults) and given every other output (output faults), tried at margin 0; and 100
  distinct mutants per model of one and of two hidden states, a copy of a state that `observe()`
  reports under the original's name but that differs on one cell, tried at margin 1.
- **The expected verdict** comes from the theory, computed by code that shares none of `spine`'s:
  a mutant that some input sequence tells from the spine, and whose smallest equivalent machine
  has at most K more states than the spine under names the spine has, is *covered*, and conform
  must kill it. A state under a name the spine lacks diverges the moment `observe()` reports it,
  so it needs no reaching and isn't counted. A mutant that no sequence tells from the spine is the
  *same*, and conform must pass it. The rest are *beyond* the guarantee, and are counted.

## The models

The files, their sources, papers and licence are in
[`tools/internal/benchmarks/testdata/`](../tools/internal/benchmarks/testdata/README.md).

| Model | States | States, Moore fold | Inputs | Outputs | Cells |
|---|---|---|---|---|---|
| Coffee machine | 6 | 7 | 4 | 3 | 24 |
| ASN bank card | 6 | 14 | 14 | 10 | 84 |
| mbedTLS 2.0.0 server | 6 | 8 | 11 | 6 | 66 |
| Mosquitto broker | 10 | 39 | 11 | 11 | 110 |

### Coffee machine

![The coffee machine: from s0, WATER leads to s4 and POD to s2; the other of the two then leads
to s3, where BUTTON makes coffee and leads to s5. CLEAN returns every state but s1 to s0. BUTTON
without both water and a pod, or any input but CLEAN after the coffee, leads to s1, which answers
error to everything.](benchmarks/1-coffee-machine.svg)

The running example of Steffen, Howar and Merten (2011): small enough to read whole, with one
state, s1, that nothing leaves.

### ASN bank card

![The ASN bank card's Maestro application: SELECT leads from s0 to s1, GPO from s1 to s2, and
from s2 the first GENERATE AC leads to s3, s4 or s5 by the cryptogram asked for; nearly every
other input returns to s1 with a status word. A key below the drawing gives the abbreviations.](benchmarks/2-bank-card.svg)

Learned from a real card by Aarts, de Ruiter and Poll (2013). Its file marks no start; `s0` is
the start, the only state from which every other can be reached. Its 14 inputs are the widest
alphabet of the four, and it has no sink.

### mbedTLS 2.0.0 as a TLS 1.0 server

![mbedTLS 2.0.0 as a TLS 1.0 server: ClientHello, RSAClientKeyExchange, ChangeCipherSpec and
Finished lead from s0 through s2, s3 and s4 to s5, the handshake done, where a second ClientHello
is refused with a warning. Every other input closes the connection, leading to s1; those edges
are not drawn.](benchmarks/3-tls.svg)

Learned by Janssen (2021) for fingerprinting TLS implementations. The edges into s1, the closed
connection, are left out of the drawing: from every other state, each input no drawn edge names
leads there, as the note in the drawing says.

### Mosquitto MQTT broker, one client

![Mosquitto with one client: Connect leads from s0 to s1. The eight states s1 and s3 to s9 form
a cube, drawn as an outer and an inner square: Subscribe and UnSubScribe move left and right,
SubscribeRetained and UnSubScribeRetained up and down, PublishRetain and DeleteRetained between
the squares. A table gives the inputs that stay in each state. Connect, Disconnect and
DisconnectTCP lead every state of the cube to s2, which closes every connection; those edges are
not drawn.](benchmarks/4-mqtt.svg)

Learned by Tappler, Aichernig and Bloem (2017). Its eight connected states are every combination
of three facts: the client is subscribed, it is subscribed to retained messages, and the broker
holds a retained message. That makes a cube, drawn flat; the inputs that leave a state where it is
are in the table, and the edges into s2, the sink, are left out, as the note says. It grows the
most under the fold, from 10 states to 39.

## Results

Of 9,796 conform runs, 3.9 million sequences in 3 min 25 s:

- **Every covered mutant was killed:** 6,542, in both translations and every class.
- **Every mutant the same as its spine passed:** 2,736. Conform raised no false alarm.
- **With outputs dropped, all 2,234 output faults go unseen,** as they must: a spine without
  outputs can't tell them apart. Under the Moore fold all 2,234 are covered, and were killed. This
  is how much a spine that states outputs would add.
- **Two hidden states are mostly beyond margin 1,** and 431 of them survived, as ADR 0008 says
  they may: 149 of 162 with outputs dropped and 282 of 354 under the fold.

Four deliberate breaks of conform, each in a copy of `tools/`, were each caught by the sample that
plain `go test` runs: trying one step fewer than the margin, one step more, ignoring a wrong state
after a run's last step, and reporting a refusal where the spine stays.

| Model | Translation | Class | Margin | Mutants | Covered, killed | Same, passed | Beyond, killed | Sequences | Time |
|---|---|---|---|---|---|---|---|---|---|
| coffee | outputs dropped | none | 1 | 1 | 0, 0 | 1, 1 | 0, 0 | 100 | 51ms |
| coffee | outputs dropped | transfer | 0 | 120 | 120, 120 | 0, 0 | 0, 0 | 2880 | 3.264s |
| coffee | outputs dropped | output | 0 | 48 | 0, 0 | 48, 48 | 0, 0 | 1152 | 1.269s |
| coffee | outputs dropped | hidden-1 | 1 | 100 | 51, 51 | 49, 49 | 0, 0 | 9932 | 6.405s |
| coffee | outputs dropped | hidden-2 | 1 | 100 | 2, 2 | 53, 53 | 45, 8 | 10000 | 6.325s |
| coffee | Moore fold | none | 1 | 1 | 0, 0 | 1, 1 | 0, 0 | 116 | 58ms |
| coffee | Moore fold | transfer | 0 | 120 | 120, 120 | 0, 0 | 0, 0 | 3360 | 3.361s |
| coffee | Moore fold | output | 0 | 48 | 48, 48 | 0, 0 | 0, 0 | 1344 | 1.383s |
| coffee | Moore fold | hidden-1 | 1 | 100 | 94, 94 | 6, 6 | 0, 0 | 11456 | 6.519s |
| coffee | Moore fold | hidden-2 | 1 | 100 | 4, 4 | 7, 7 | 89, 21 | 11600 | 6.505s |
| bankcard | outputs dropped | none | 1 | 1 | 0, 0 | 1, 1 | 0, 0 | 1190 | 648ms |
| bankcard | outputs dropped | transfer | 0 | 420 | 420, 420 | 0, 0 | 0, 0 | 35280 | 23.659s |
| bankcard | outputs dropped | output | 0 | 756 | 0, 0 | 756, 756 | 0, 0 | 63504 | 42.783s |
| bankcard | outputs dropped | hidden-1 | 1 | 100 | 48, 48 | 52, 52 | 0, 0 | 118986 | 1m18.529s |
| bankcard | outputs dropped | hidden-2 | 1 | 100 | 0, 0 | 60, 60 | 40, 2 | 119000 | 1m25.392s |
| bankcard | Moore fold | none | 1 | 1 | 0, 0 | 1, 1 | 0, 0 | 2758 | 2.147s |
| bankcard | Moore fold | transfer | 0 | 420 | 420, 420 | 0, 0 | 0, 0 | 82320 | 55.109s |
| bankcard | Moore fold | output | 0 | 756 | 756, 756 | 0, 0 | 0, 0 | 148176 | 1m35.701s |
| bankcard | Moore fold | hidden-1 | 1 | 100 | 94, 94 | 4, 4 | 2, 2 | 275478 | 3m18.245s |
| bankcard | Moore fold | hidden-2 | 1 | 100 | 0, 0 | 8, 8 | 92, 15 | 275786 | 3m26.907s |
| tls | outputs dropped | none | 1 | 1 | 0, 0 | 1, 1 | 0, 0 | 737 | 526ms |
| tls | outputs dropped | transfer | 0 | 330 | 330, 330 | 0, 0 | 0, 0 | 21780 | 16.64s |
| tls | outputs dropped | output | 0 | 330 | 0, 0 | 330, 330 | 0, 0 | 21780 | 17.205s |
| tls | outputs dropped | hidden-1 | 1 | 100 | 50, 50 | 50, 50 | 0, 0 | 73645 | 48.947s |
| tls | outputs dropped | hidden-2 | 1 | 100 | 1, 1 | 67, 67 | 32, 2 | 73689 | 48.8s |
| tls | Moore fold | none | 1 | 1 | 0, 0 | 1, 1 | 0, 0 | 979 | 722ms |
| tls | Moore fold | transfer | 0 | 330 | 330, 330 | 0, 0 | 0, 0 | 29040 | 21.443s |
| tls | Moore fold | output | 0 | 330 | 330, 330 | 0, 0 | 0, 0 | 29040 | 22.477s |
| tls | Moore fold | hidden-1 | 1 | 100 | 95, 95 | 5, 5 | 0, 0 | 97812 | 1m14.936s |
| tls | Moore fold | hidden-2 | 1 | 100 | 8, 8 | 13, 13 | 79, 6 | 97856 | 1m22.804s |
| mqtt | outputs dropped | none | 1 | 1 | 0, 0 | 1, 1 | 0, 0 | 1221 | 851ms |
| mqtt | outputs dropped | transfer | 0 | 990 | 990, 990 | 0, 0 | 0, 0 | 108900 | 1m26.799s |
| mqtt | outputs dropped | output | 0 | 1100 | 0, 0 | 1100, 1100 | 0, 0 | 121000 | 1m42.258s |
| mqtt | outputs dropped | hidden-1 | 1 | 100 | 45, 45 | 55, 55 | 0, 0 | 122100 | 1m35.56s |
| mqtt | outputs dropped | hidden-2 | 1 | 100 | 0, 0 | 55, 55 | 45, 1 | 122100 | 1m51.442s |
| mqtt | Moore fold | none | 1 | 1 | 0, 0 | 1, 1 | 0, 0 | 4730 | 5.394s |
| mqtt | Moore fold | transfer | 0 | 990 | 990, 990 | 0, 0 | 0, 0 | 424710 | 6m20.513s |
| mqtt | Moore fold | output | 0 | 1100 | 1100, 1100 | 0, 0 | 0, 0 | 471900 | 6m2s |
| mqtt | Moore fold | hidden-1 | 1 | 100 | 94, 94 | 6, 6 | 0, 0 | 472461 | 7m35.934s |
| mqtt | Moore fold | hidden-2 | 1 | 100 | 2, 2 | 4, 4 | 94, 30 | 472978 | 7m20.594s |

*None* is the model itself as the code, which must pass with every case tried. *Time* is the sum
of each conform run's own time, so a row can take longer than the whole run, which spreads the
runs over the cores.

## Not covered

What #106 left out, among them refusals, margins above 1, larger models, data guards and
comparing two implementations' models, is listed in the issue under "Not done here".

## Provenance

The drawings were drawn from the `.dot` files on 2026-11-08. Before each was written, every edge
label, the table and the notes were expanded back into cells and compared with the model's, cell
for cell; each was then rendered in light and dark and looked at.
