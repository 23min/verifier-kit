# The models

Four Mealy machines from the Automata Wiki, <https://automata.cs.ru.nl/>, at Radboud University,
for the benchmarks (#106, `docs/benchmarks.md`). Each `.dot` file is as the wiki served it on
2026-11-08, unchanged.

| File | Benchmark | Paper |
|---|---|---|
| `coffeemachine.dot` | [CoffeeMachine-SteffenEtAl2011](https://automata.cs.ru.nl/BenchmarkCoffeeMachine-SteffenEtAl2011/Mealy) | Bernhard Steffen, Falk Howar and Maik Merten, Introduction to Active Automata Learning from a Practical Perspective, SFM 2011, LNCS 6659, pages 256-296 |
| `ASN_learnresult_MAESTRO_fix.dot` | [Bankcard-AartsEtAl2013](https://automata.cs.ru.nl/BenchmarkBankcard-AartsEtAl2013/Mealy) | F. Aarts, J. de Ruiter and E. Poll, Formal models of bank cards for free, SECTEST 2013 |
| `tls-1.0-mbedtls-2.0.0.dot` | [TLS-Janssen2021](https://automata.cs.ru.nl/BenchmarkTLS-Janssen2021/Mealy) | Erwin Janssen, Fingerprinting TLS Implementations Using Model Learning, master's thesis, 2021 |
| `mosquitto__single_client.dot` | [MQTT-TapplerEtAl2017](https://automata.cs.ru.nl/BenchmarkMQTT-TapplerEtAl2017/Mealy) | Martin Tappler, Bernhard K. Aichernig and Roderick Bloem, Model-Based Testing IoT Communication via Active Automata Learning, ICST 2017 |

The bank card's file marks no start. Its `s0` is the start: it is the only state from which
every other can be reached.

## Licence

The wiki's home page says it set up "a publicly (by MIT licence) available set of benchmarks".
It names no copyright holder; each model's authors are those of its paper, above. The MIT
licence's terms:

> Permission is hereby granted, free of charge, to any person obtaining a copy of this software
> and associated documentation files (the "Software"), to deal in the Software without
> restriction, including without limitation the rights to use, copy, modify, merge, publish,
> distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the
> Software is furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all copies or
> substantial portions of the Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING
> BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
> NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
> DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
> OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
