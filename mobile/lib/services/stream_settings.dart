import 'package:shared_preferences/shared_preferences.dart';

/// Persists the RTMP Server + Stream Key using the exact same convention as
/// OBS and the panel's own frontend (frontend/src/utils/streaming.ts):
/// Server is the bare rtmp://host:port endpoint, Stream Key is the relay
/// secret, and OBS joins them with "/" into the actual publish path. Keeping
/// the same split here means a streamer copies the exact same two values
/// from the server's page in the panel, no translation needed.
class StreamSettings {
  static const _serverKey = 'rtmp_server';
  static const _streamKeyPrefKey = 'rtmp_stream_key';

  final String server;
  final String streamKey;

  const StreamSettings({required this.server, required this.streamKey});

  bool get isComplete => server.trim().isNotEmpty && streamKey.trim().isNotEmpty;

  String get publishUrl {
    final s = server.trim().replaceAll(RegExp(r'/+$'), '');
    final k = streamKey.trim().replaceAll(RegExp(r'^/+'), '');
    return '$s/$k';
  }

  static Future<StreamSettings> load() async {
    final prefs = await SharedPreferences.getInstance();
    return StreamSettings(
      server: prefs.getString(_serverKey) ?? '',
      streamKey: prefs.getString(_streamKeyPrefKey) ?? '',
    );
  }

  Future<void> save() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_serverKey, server);
    await prefs.setString(_streamKeyPrefKey, streamKey);
  }
}
