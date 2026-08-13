import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:powernode_irl/main.dart';
import 'package:powernode_irl/screens/settings_screen.dart';

void main() {
  testWidgets('starts on the settings screen with no saved server/key', (tester) async {
    SharedPreferences.setMockInitialValues({});

    await tester.pumpWidget(const PowerNodeIrlApp());
    await tester.pumpAndSettle();

    expect(find.byType(SettingsScreen), findsOneWidget);
  });
}
