import 'package:flutter/material.dart';

import 'screens/settings_screen.dart';
import 'screens/stream_screen.dart';
import 'services/stream_settings.dart';

void main() {
  runApp(const PowerNodeIrlApp());
}

class PowerNodeIrlApp extends StatelessWidget {
  const PowerNodeIrlApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'PowerNode IRL',
      theme: ThemeData(
        brightness: Brightness.dark,
        colorScheme: ColorScheme.fromSeed(
          seedColor: Colors.pinkAccent,
          brightness: Brightness.dark,
        ),
        useMaterial3: true,
      ),
      home: const _Root(),
    );
  }
}

/// Skips straight to the stream screen if a server/key are already saved
/// from a previous run, otherwise starts at settings.
class _Root extends StatelessWidget {
  const _Root();

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<StreamSettings>(
      future: StreamSettings.load(),
      builder: (context, snapshot) {
        if (!snapshot.hasData) {
          return const Scaffold(body: Center(child: CircularProgressIndicator()));
        }
        return snapshot.data!.isComplete ? const StreamScreen() : const SettingsScreen();
      },
    );
  }
}
