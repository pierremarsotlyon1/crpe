package fr.prepamathscrpe.qa;

import android.app.Activity;
import android.app.Instrumentation;
import android.graphics.Rect;
import android.os.Bundle;
import android.os.SystemClock;
import android.util.Base64;
import android.view.accessibility.AccessibilityNodeInfo;
import java.nio.charset.StandardCharsets;
import org.json.JSONArray;
import org.json.JSONObject;

// Separate test APK. Read the current accessibility tree without waiting for a countdown to stop.
public final class Tree extends Instrumentation {
  @Override public void onCreate(Bundle arguments) { super.onCreate(arguments); start(); }
  @Override public void onStart() {
    Bundle result = new Bundle();
    try {
      AccessibilityNodeInfo root;
      long deadline = SystemClock.uptimeMillis() + 1800;
      do {
        root = getUiAutomation().getRootInActiveWindow();
        if (root == null) SystemClock.sleep(75);
      } while (root == null && SystemClock.uptimeMillis() < deadline);
      JSONArray visible = new JSONArray();
      collect(root, visible);
      result.putString("tree", Base64.encodeToString(visible.toString().getBytes(StandardCharsets.UTF_8), Base64.NO_WRAP));
      finish(Activity.RESULT_OK, result);
    } catch (Exception e) { finish(Activity.RESULT_CANCELED, result); }
  }
  private static void collect(AccessibilityNodeInfo node, JSONArray list) throws Exception {
    if (node == null) return;
    if (node.isVisibleToUser()) {
      Rect r = new Rect(); node.getBoundsInScreen(r);
      JSONObject item = new JSONObject();
      item.put("text", node.getText() == null ? "" : node.getText().toString());
      item.put("content-desc", node.getContentDescription() == null ? "" : node.getContentDescription().toString());
      item.put("bounds", r.toShortString());
      list.put(item);
    }
    for (int i = 0; i < node.getChildCount(); i++) collect(node.getChild(i), list);
  }
}
