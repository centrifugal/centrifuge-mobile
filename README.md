This library allows to generate Centrifugo client libraries for iOS and Android using [gomobile](https://github.com/golang/mobile/) tool.

This is **experimental** and have not been tested yet at all.

See [examples](https://github.com/centrifugal/centrifuge-mobile/tree/master/examples) to dive into.

[API documentation on Godoc](https://godoc.org/github.com/centrifugal/centrifuge-mobile)

Build for mobile:

```
gomobile bind -target=ios -tags="mobile" github.com/centrifugal/centrifuge-mobile
gomobile bind -target=android -tags="mobile" github.com/centrifugal/centrifuge-mobile
```
