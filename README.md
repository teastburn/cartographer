# cartographer
> A real-time map of geo coordinates designed for high loads.

This is an example project that updates a map in real time with millions of geocoordinates per day.

## Installation

OS X & Linux:

```sh
# install docker 17+
# install go 1.8+
go run $GOROOT/src/crypto/tls/generate_cert.go --host localhost # writes cert.pem, key.pem
make dev
# available on https://localhost:8080 by default
```

## Usage example

```sh
# open https://localhost:8080/static/index.html in browser
make cities
# watch map update in browser
```

## Development setup

TODO

## Dependencies

* Go 1.8.3+
* Glide 0.12.3+
* Echo 3.2.1+
* Docker 17+

## Release History

* 0.0.1
    * Work in progress

## TODO

* Scale bottlenecks:
  * Number of websocket listeners -- move to distributed pubsub model
* Better docs
* Make JS not so 2000s
* Input validation
* Socket origin restrictions
* Unit test concurrency model

## Meta

[Tristan Eastburn](https://www.linkedin.com/in/teastburn/)

Distributed under the GPL3. See ``LICENSE`` for more information.

[https://github.com/teastburn](https://github.com/teastburn)

## Contributing

1. Fork it (<https://github.com/teastburn/cartographer/fork>)
2. Create your feature branch (`git checkout -b feature/fooBar`)
3. Commit your changes (`git commit -am 'Add some fooBar'`)
4. Push to the branch (`git push origin feature/fooBar`)
5. Create a new Pull Request

<!-- Markdown link & img dfn's -->
[npm-image]: https://img.shields.io/npm/v/datadog-metrics.svg?style=flat-square
[npm-url]: https://npmjs.org/package/datadog-metrics
[npm-downloads]: https://img.shields.io/npm/dm/datadog-metrics.svg?style=flat-square
[travis-image]: https://img.shields.io/travis/dbader/node-datadog-metrics/master.svg?style=flat-square
[travis-url]: https://travis-ci.org/dbader/node-datadog-metrics
[wiki]: https://github.com/yourname/yourproject/wiki
