import 'dart:async';

import 'package:flutter/material.dart';
import 'package:permission_handler/permission_handler.dart';
import 'package:rtmp_broadcaster/camera.dart';
import 'package:wakelock_plus/wakelock_plus.dart';

import '../services/stream_settings.dart';
import 'settings_screen.dart';

class StreamScreen extends StatefulWidget {
  const StreamScreen({super.key});

  @override
  State<StreamScreen> createState() => _StreamScreenState();
}

enum _Status { initializing, ready, live, error }

class _StreamScreenState extends State<StreamScreen> with WidgetsBindingObserver {
  CameraController? _controller;
  _Status _status = _Status.initializing;
  String? _error;
  StreamSettings? _settings;
  DateTime? _liveSince;
  Timer? _ticker;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _bootstrap();
  }

  Future<void> _bootstrap() async {
    final settings = await StreamSettings.load();
    if (!settings.isComplete) {
      if (!mounted) return;
      Navigator.of(context).pushReplacement(
        MaterialPageRoute(builder: (_) => const SettingsScreen()),
      );
      return;
    }
    _settings = settings;

    final camStatus = await Permission.camera.request();
    final micStatus = await Permission.microphone.request();
    if (!camStatus.isGranted || !micStatus.isGranted) {
      setState(() {
        _status = _Status.error;
        _error = 'Нужен доступ к камере и микрофону, чтобы стримить.';
      });
      return;
    }

    try {
      final cameras = await availableCameras();
      if (cameras.isEmpty) {
        setState(() {
          _status = _Status.error;
          _error = 'Камера не найдена.';
        });
        return;
      }
      final back = cameras.firstWhere(
        (c) => c.lensDirection == CameraLensDirection.back,
        orElse: () => cameras.first,
      );
      final controller = CameraController(
        back,
        ResolutionPreset.high,
        enableAudio: true,
      );
      await controller.initialize();
      if (!mounted) return;
      setState(() {
        _controller = controller;
        _status = _Status.ready;
      });
    } catch (e) {
      setState(() {
        _status = _Status.error;
        _error = 'Не удалось запустить камеру: $e';
      });
    }
  }

  Future<void> _goLive() async {
    final controller = _controller;
    final settings = _settings;
    if (controller == null || settings == null) return;
    try {
      await controller.startVideoStreaming(settings.publishUrl);
      await WakelockPlus.enable();
      _ticker?.cancel();
      _ticker = Timer.periodic(const Duration(seconds: 1), (_) => setState(() {}));
      setState(() {
        _status = _Status.live;
        _liveSince = DateTime.now();
      });
    } catch (e) {
      setState(() {
        _status = _Status.error;
        _error = 'Не удалось начать трансляцию: $e';
      });
    }
  }

  Future<void> _stop() async {
    final controller = _controller;
    if (controller == null) return;
    try {
      await controller.stopVideoStreaming();
    } catch (_) {
      // Already stopped or never fully started -- nothing more to do.
    }
    await WakelockPlus.disable();
    _ticker?.cancel();
    _ticker = null;
    setState(() {
      _status = _Status.ready;
      _liveSince = null;
    });
  }

  String _formatElapsed(Duration d) {
    String two(int n) => n.toString().padLeft(2, '0');
    final h = two(d.inHours);
    final m = two(d.inMinutes.remainder(60));
    final s = two(d.inSeconds.remainder(60));
    return d.inHours > 0 ? '$h:$m:$s' : '$m:$s';
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _controller?.dispose();
    _ticker?.cancel();
    WakelockPlus.disable();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    // Losing the camera to the OS mid-stream (call, app switch) would
    // otherwise leave _status stuck on "live" while nothing is actually
    // being published -- stop cleanly instead of pretending it's fine.
    if (state != AppLifecycleState.resumed && _status == _Status.live) {
      _stop();
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        title: const Text('PowerNode IRL'),
        actions: [
          IconButton(
            icon: const Icon(Icons.settings),
            onPressed: _status == _Status.live
                ? null
                : () => Navigator.of(context).push(
                      MaterialPageRoute(builder: (_) => const SettingsScreen()),
                    ),
          ),
        ],
      ),
      body: _buildBody(),
    );
  }

  Widget _buildBody() {
    switch (_status) {
      case _Status.initializing:
        return const Center(child: CircularProgressIndicator());
      case _Status.error:
        return Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(Icons.error_outline, color: Colors.redAccent, size: 40),
                const SizedBox(height: 12),
                Text(
                  _error ?? 'Что-то пошло не так.',
                  textAlign: TextAlign.center,
                  style: const TextStyle(color: Colors.white),
                ),
                const SizedBox(height: 20),
                FilledButton(onPressed: _bootstrap, child: const Text('Повторить')),
              ],
            ),
          ),
        );
      case _Status.ready:
      case _Status.live:
        final controller = _controller;
        if (controller == null || controller.value.isInitialized != true) {
          return const Center(child: CircularProgressIndicator());
        }
        return Stack(
          fit: StackFit.expand,
          children: [
            Center(
              child: AspectRatio(
                aspectRatio: controller.value.aspectRatio,
                child: CameraPreview(controller),
              ),
            ),
            if (_status == _Status.live)
              Positioned(
                top: 16,
                left: 16,
                child: Container(
                  padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
                  decoration: BoxDecoration(
                    color: Colors.red,
                    borderRadius: BorderRadius.circular(6),
                  ),
                  child: Text(
                    _liveSince == null
                        ? 'LIVE'
                        : 'LIVE ${_formatElapsed(DateTime.now().difference(_liveSince!))}',
                    style: const TextStyle(color: Colors.white, fontWeight: FontWeight.bold),
                  ),
                ),
              ),
            Positioned(
              bottom: 32,
              left: 0,
              right: 0,
              child: Center(
                child: FilledButton(
                  style: FilledButton.styleFrom(
                    backgroundColor: _status == _Status.live ? Colors.red : Colors.pinkAccent,
                    padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 16),
                  ),
                  onPressed: _status == _Status.live ? _stop : _goLive,
                  child: Text(_status == _Status.live ? 'Остановить' : 'В эфир'),
                ),
              ),
            ),
          ],
        );
    }
  }
}
