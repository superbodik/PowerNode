# PowerNode IRL

Flutter app for streaming straight from a phone's camera to a PowerNode
RTMP relay server — no OBS, no laptop, just the phone. Uses the same
Server + Stream Key convention as OBS (see the RTMP Relay egg's server
page in the panel): Server is the bare `rtmp://host:port` endpoint, Stream
Key is the relay secret, joined into the actual publish URL the same way
OBS does it internally.

## Status

Built and verified to **compile** (`flutter analyze` clean, `flutter test`
passing, `flutter build apk --debug` producing a real installable APK) --
not yet verified end-to-end on a real device against a live relay, since
that needs an actual phone with a camera and network, neither of which
exist in the environment this was written in. Test on a real device
before relying on it for an actual stream.

## Stack

- Flutter 3.41+ / Dart 3.11+
- [`rtmp_broadcaster`](https://pub.dev/packages/rtmp_broadcaster) for
  camera capture + RTMP publish (wraps RootEncoder on Android, HaishinKit
  on iOS) -- picked after checking pub.dev directly for an actively
  maintained, Dart-3-compatible option; the older `camera_with_rtmp` is
  Dart 3 incompatible and unmaintained for 6 years.
- `shared_preferences` for persisting Server/Stream Key between launches
- `permission_handler` for camera/mic runtime permissions
- `wakelock_plus` to keep the screen on while live

## Run locally

```bash
cd mobile
flutter pub get
flutter run   # needs a connected device or emulator
```

## Build

```bash
flutter build apk --release   # Android
flutter build ios --release   # iOS, needs a Mac + Xcode
```

## Known rough edges

- Android needed `android.enableJetifier=true` in `android/gradle.properties`
  -- one of `rtmp_broadcaster`'s transitive dependencies still ships the
  old `com.android.support` library, which otherwise conflicts with
  AndroidX at manifest-merge time.
- No account/login system -- the Server + Stream Key are exactly what you'd
  paste into OBS, copied from the server's page in the panel. Anyone with
  those two values can stream to that relay, same trust model as OBS.
- Front/back camera switching, bitrate/resolution settings, and a proper
  "reconnect on dropped connection" story aren't built yet -- v1 is
  deliberately minimal: point camera, go live, stop.
