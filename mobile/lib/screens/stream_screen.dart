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
  List<CameraDescription> _cameras = [];
  CameraDescription? _activeCamera;
  bool _micEnabled = true;
  bool _paused = false;
  bool _busySwitching = false;

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
      _cameras = await availableCameras();
      if (_cameras.isEmpty) {
        setState(() {
          _status = _Status.error;
          _error = 'Камера не найдена.';
        });
        return;
      }
      final back = _cameras.firstWhere(
        (c) => c.lensDirection == CameraLensDirection.back,
        orElse: () => _cameras.first,
      );
      await _openCamera(back, micEnabled: true);
      if (!mounted) return;
      setState(() => _status = _Status.ready);
    } catch (e) {
      setState(() {
        _status = _Status.error;
        _error = 'Не удалось запустить камеру: $e';
      });
    }
  }

  /// Creates a fresh CameraController for [description] and swaps it in for
  /// whatever was there before. rtmp_broadcaster ties enableAudio and the
  /// camera to the controller at construction time -- there's no "just mute
  /// the mic" or "just switch lenses" call on a live controller, so both
  /// the flip-camera and mute-mic buttons go through this same rebuild.
  Future<void> _openCamera(CameraDescription description, {required bool micEnabled}) async {
    final old = _controller;
    final controller = CameraController(
      description,
      ResolutionPreset.high,
      enableAudio: micEnabled,
      // See _goLive's comment on streamingPreset -- same reasoning here,
      // this just also applies across camera/mic switches.
      androidUseOpenGL: true,
      streamingPreset: ResolutionPreset.medium,
    );
    await controller.initialize();
    await old?.dispose();
    if (!mounted) {
      await controller.dispose();
      return;
    }
    setState(() {
      _controller = controller;
      _activeCamera = description;
      _micEnabled = micEnabled;
    });
  }

  CameraDescription? _otherCamera() {
    if (_activeCamera == null || _cameras.length < 2) return null;
    return _cameras.firstWhere(
      (c) => c.lensDirection != _activeCamera!.lensDirection,
      orElse: () => _activeCamera!,
    );
  }

  /// Switching camera or mic mid-stream means tearing down and recreating
  /// the controller, which drops the RTMP connection -- so this stops,
  /// swaps, and immediately republishes rather than leaving the viewer on
  /// a frozen frame indefinitely. A brief reconnect glitch on the viewer's
  /// end is the honest cost of switching hardware inputs live; there's no
  /// way to do it seamlessly with this package's API.
  Future<void> _flipCamera() async {
    final next = _otherCamera();
    if (next == null || _busySwitching) return;
    await _switchTo(next, _micEnabled);
  }

  Future<void> _toggleMic() async {
    if (_activeCamera == null || _busySwitching) return;
    await _switchTo(_activeCamera!, !_micEnabled);
  }

  Future<void> _switchTo(CameraDescription description, bool micEnabled) async {
    setState(() => _busySwitching = true);
    final wasLive = _status == _Status.live;
    final settings = _settings;
    try {
      if (wasLive) {
        await _controller?.stopVideoStreaming();
      }
      await _openCamera(description, micEnabled: micEnabled);
      if (wasLive && settings != null) {
        await _controller!.startVideoStreaming(settings.publishUrl, bitrate: 2500 * 1024);
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Не удалось переключить: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _busySwitching = false);
    }
  }

  /// The package only exposes pausing/resuming everything together --
  /// there's no separate "keep sending audio, stop sending video" call, so
  /// this pauses both rather than pretending to mute just the camera.
  Future<void> _togglePause() async {
    final controller = _controller;
    if (controller == null || _status != _Status.live) return;
    try {
      if (_paused) {
        await controller.resumeVideoStreaming();
      } else {
        await controller.pauseVideoStreaming();
      }
      setState(() => _paused = !_paused);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Не удалось поставить на паузу: $e')),
        );
      }
    }
  }

  Future<void> _goLive() async {
    final controller = _controller;
    final settings = _settings;
    if (controller == null || settings == null) return;
    try {
      // 2500kbps at the medium streamingPreset -- matched to a lighter
      // encode load than the package's 1200kbps default assumed for
      // whatever resolution it's driving, explicit rather than implicit
      // so it's obvious where to tune it if a phone still struggles.
      await controller.startVideoStreaming(settings.publishUrl, bitrate: 2500 * 1024);
      await WakelockPlus.enable();
      _ticker?.cancel();
      _ticker = Timer.periodic(const Duration(seconds: 1), (_) => setState(() {}));
      setState(() {
        _status = _Status.live;
        _liveSince = DateTime.now();
        _paused = false;
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
      _paused = false;
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
    // No explicit orientation lock anywhere in this app or its Android
    // manifest, so the device's own rotation (and the camera plugin's own
    // sensor-orientation handling) drives the frame orientation -- this
    // screen just needs to not fight that, not implement it itself.
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
                    color: _paused ? Colors.grey.shade700 : Colors.red,
                    borderRadius: BorderRadius.circular(6),
                  ),
                  child: Text(
                    _paused
                        ? 'ПАУЗА'
                        : (_liveSince == null
                            ? 'LIVE'
                            : 'LIVE ${_formatElapsed(DateTime.now().difference(_liveSince!))}'),
                    style: const TextStyle(color: Colors.white, fontWeight: FontWeight.bold),
                  ),
                ),
              ),
            // Camera flip / mic mute -- available whenever a camera is up,
            // live or not, since deciding "front cam, mic off" before going
            // live is just as normal as changing it mid-stream.
            Positioned(
              top: 16,
              right: 16,
              child: Column(
                children: [
                  _RoundIconButton(
                    icon: Icons.cameraswitch,
                    active: false,
                    enabled: !_busySwitching && _cameras.length > 1,
                    onTap: _flipCamera,
                    tooltip: 'Переключить камеру',
                  ),
                  const SizedBox(height: 10),
                  _RoundIconButton(
                    icon: _micEnabled ? Icons.mic : Icons.mic_off,
                    active: !_micEnabled,
                    enabled: !_busySwitching,
                    onTap: _toggleMic,
                    tooltip: _micEnabled ? 'Выключить микрофон' : 'Включить микрофон',
                  ),
                  if (_status == _Status.live) ...[
                    const SizedBox(height: 10),
                    _RoundIconButton(
                      icon: _paused ? Icons.play_arrow : Icons.pause,
                      active: _paused,
                      enabled: true,
                      onTap: _togglePause,
                      // Honest label: the package can only pause/resume
                      // audio+video together, there's no video-only mute.
                      tooltip: _paused ? 'Продолжить (аудио и видео)' : 'Пауза (аудио и видео)',
                    ),
                  ],
                ],
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

class _RoundIconButton extends StatelessWidget {
  const _RoundIconButton({
    required this.icon,
    required this.active,
    required this.enabled,
    required this.onTap,
    required this.tooltip,
  });

  final IconData icon;
  final bool active;
  final bool enabled;
  final VoidCallback onTap;
  final String tooltip;

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: tooltip,
      child: Material(
        color: active ? Colors.redAccent : Colors.black45,
        shape: const CircleBorder(),
        child: InkWell(
          customBorder: const CircleBorder(),
          onTap: enabled ? onTap : null,
          child: Padding(
            padding: const EdgeInsets.all(10),
            child: Icon(
              icon,
              color: enabled ? Colors.white : Colors.white38,
              size: 22,
            ),
          ),
        ),
      ),
    );
  }
}
