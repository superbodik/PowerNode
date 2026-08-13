import 'package:flutter/material.dart';

import '../services/stream_settings.dart';
import 'stream_screen.dart';

class SettingsScreen extends StatefulWidget {
  const SettingsScreen({super.key});

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  final _serverController = TextEditingController();
  final _keyController = TextEditingController();
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    StreamSettings.load().then((s) {
      _serverController.text = s.server;
      _keyController.text = s.streamKey;
      setState(() => _loading = false);
    });
  }

  @override
  void dispose() {
    _serverController.dispose();
    _keyController.dispose();
    super.dispose();
  }

  Future<void> _saveAndContinue() async {
    final settings = StreamSettings(
      server: _serverController.text,
      streamKey: _keyController.text,
    );
    if (!settings.isComplete) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Заполни оба поля')),
      );
      return;
    }
    await settings.save();
    if (!mounted) return;
    Navigator.of(context).pushReplacement(
      MaterialPageRoute(builder: (_) => const StreamScreen()),
    );
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    return Scaffold(
      appBar: AppBar(title: const Text('PowerNode IRL')),
      body: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Те же самые Server и Stream Key, что показаны на странице сервера в панели для OBS.',
              style: TextStyle(color: Colors.grey),
            ),
            const SizedBox(height: 24),
            TextField(
              controller: _serverController,
              decoration: const InputDecoration(
                labelText: 'Server',
                hintText: 'rtmp://your-panel-host:1935',
                border: OutlineInputBorder(),
              ),
              keyboardType: TextInputType.url,
            ),
            const SizedBox(height: 16),
            TextField(
              controller: _keyController,
              decoration: const InputDecoration(
                labelText: 'Stream Key',
                hintText: 'RELAY_SECRET из панели',
                border: OutlineInputBorder(),
              ),
              obscureText: true,
            ),
            const SizedBox(height: 32),
            SizedBox(
              width: double.infinity,
              child: FilledButton(
                onPressed: _saveAndContinue,
                child: const Padding(
                  padding: EdgeInsets.symmetric(vertical: 14),
                  child: Text('Продолжить'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
