This library allows to generate Centrifugo client libraries for iOS and Android using [gomobile](https://github.com/golang/mobile/) tool. Also it can be used directly from Go applications.

See [examples](https://github.com/centrifugal/centrifuge-mobile/tree/master/examples) to dive into. In that folder you can find how to use this library from Go, also example [iOS app](https://github.com/centrifugal/centrifuge-mobile/tree/master/examples/ios/CentrifugoIOS) and [Android app](https://github.com/centrifugal/centrifuge-mobile/tree/master/examples/android/CentrifugoAndroid) using generated client bindings.

To build for mobile (note that we are ensuring in `mobile` build tag here):

```
gomobile bind -target=ios -tags="mobile" github.com/centrifugal/centrifuge-mobile
gomobile bind -target=android -tags="mobile" github.com/centrifugal/centrifuge-mobile
```

See how to import generated library on [iOS](https://medium.com/@fzambia/going-mobile-adapting-centrifugo-go-websocket-client-to-be-used-for-ios-and-android-app-e72dc2736f01#adb8) and on [Android](https://medium.com/@fzambia/going-mobile-adapting-centrifugo-go-websocket-client-to-be-used-for-ios-and-android-app-e72dc2736f01#.fow320d0h) in introduction post.

[API documentation on Godoc](https://godoc.org/github.com/centrifugal/centrifuge-mobile)
