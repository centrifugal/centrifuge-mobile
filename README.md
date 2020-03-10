This repo allows to generate Centrifuge/Centrifugo client libraries for iOS and Android app development using [gomobile](https://github.com/golang/mobile/) tool.

Gomobile is **experimental** project with all the ensuing circumstances.

You can download libraries for iOS and Android from [releases](https://github.com/centrifugal/centrifuge-mobile/releases) page. See [examples](https://github.com/centrifugal/centrifuge-mobile/tree/master/examples) to dive into.

**Note** that on iOS this library does not include bitcode (it's still optional on iOS but can become required at any moment). 

See how to import generated library [on iOS](https://medium.com/@fzambia/going-mobile-adapting-centrifugo-go-websocket-client-to-be-used-for-ios-and-android-app-e72dc2736f01#adb8) and [on Android](https://medium.com/@fzambia/going-mobile-adapting-centrifugo-go-websocket-client-to-be-used-for-ios-and-android-app-e72dc2736f01#.fow320d0h) – in two words you need to import `centrifuge.aar` (Android) or `Centrifuge.framework` (iOS).

[API documentation on Godoc](https://godoc.org/github.com/centrifugal/centrifuge-mobile) – this can be useful as iOS and Android code generated from Go code so you can rely on that docs when hacking with library.

For contributors
----------------

To build mobile libraries from Go source code using `gomobile`:

```
go get -u golang.org/x/mobile/cmd/gomobile
gomobile init
gomobile bind -target=ios github.com/centrifugal/centrifuge-mobile
gomobile bind -target=android github.com/centrifugal/centrifuge-mobile
```

Release
-------

```
go get -u golang.org/x/mobile/cmd/gomobile
gomobile init
export ANDROID_NDK_HOME=/path/to/android-ndk-rXX
make release
```
