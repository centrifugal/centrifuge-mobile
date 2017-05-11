This library allows to generate Centrifugo client libraries for iOS and Android using [gomobile](https://github.com/golang/mobile/) tool. Also it can be used directly from Go applications.

Gomobile is cool but **experimental** project so you may better use our native libraries for [iOS](https://github.com/centrifugal/centrifuge-ios) and [Android](https://github.com/centrifugal/centrifuge-android). But as they are supported by Centrifugo community members they lack some features (your contributions are really appreciated). This repo contains **full-featured** Centrifugo client for all platforms.

See [examples](https://github.com/centrifugal/centrifuge-mobile/tree/master/examples) to dive into. In that folder you can find how to use this library from Go, also example [iOS app](https://github.com/centrifugal/centrifuge-mobile/tree/master/examples/ios/CentrifugoIOS) and [Android app](https://github.com/centrifugal/centrifuge-mobile/tree/master/examples/android/CentrifugoAndroid) using generated client bindings.

See how to import generated library on [iOS](https://medium.com/@fzambia/going-mobile-adapting-centrifugo-go-websocket-client-to-be-used-for-ios-and-android-app-e72dc2736f01#adb8) and on [Android](https://medium.com/@fzambia/going-mobile-adapting-centrifugo-go-websocket-client-to-be-used-for-ios-and-android-app-e72dc2736f01#.fow320d0h) in introduction post - in two words you need to import `centrifuge.aar` (Android) or `Centrifuge.framework` (iOS) which is on top level of this repo to your studio. 

[API documentation on Godoc](https://godoc.org/github.com/centrifugal/centrifuge-mobile)

For contributors
----------------

Mobile and Go versions have some differences to be a bit more performant when using library directly from Go. So to build bindings for mobile platforms we are ensuring in `mobile` build tag this way:

```
gomobile bind -target=ios -tags="mobile" github.com/centrifugal/centrifuge-mobile
gomobile bind -target=android -tags="mobile" github.com/centrifugal/centrifuge-mobile
```

