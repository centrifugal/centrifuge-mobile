This library allows to generate Centrifuge/Centrifugo client libraries for iOS and Android using [gomobile](https://github.com/golang/mobile/) tool.

Gomobile is **experimental** project with all the ensuing circumstances.

See [examples](https://github.com/centrifugal/centrifuge-mobile/tree/master/examples) to dive into. In that folder you can find example of [iOS app](https://github.com/centrifugal/centrifuge-mobile/tree/master/examples/ios/CentrifugoIOS) and [Android app](https://github.com/centrifugal/centrifuge-mobile/tree/master/examples/android/CentrifugoAndroid) using generated client bindings.

**Note** that on iOS this library does not contain bitcode (it's still optional on iOS but can become required at any moment). 

See how to import generated library on [iOS](https://medium.com/@fzambia/going-mobile-adapting-centrifugo-go-websocket-client-to-be-used-for-ios-and-android-app-e72dc2736f01#adb8) and on [Android](https://medium.com/@fzambia/going-mobile-adapting-centrifugo-go-websocket-client-to-be-used-for-ios-and-android-app-e72dc2736f01#.fow320d0h) in introduction post - in two words you need to import `centrifuge.aar` (Android) or `Centrifuge.framework` (iOS) – see [releases](https://github.com/centrifugal/centrifuge-mobile/releases).

[API documentation on Godoc](https://godoc.org/github.com/centrifugal/centrifuge-mobile) – this can be useful as iOS and Android code generated from Go code so you can rely on that docs when hacking with library.

For contributors
----------------

To build mobile libraries from Go source code using `gomobile`:

```
go get -u golang.org/x/mobile/cmd/gomobile
gomobile init -ndk ~/PATH/TO/ANDROID/NDK
gomobile bind -target=ios github.com/centrifugal/centrifuge-mobile
gomobile bind -target=android github.com/centrifugal/centrifuge-mobile
```
