package fr.prepamathscrpe.mobile;

import android.content.Context;
import android.media.MediaRecorder;
import android.os.Build;
import java.io.File;
import java.io.IOException;
import java.io.RandomAccessFile;
import java.nio.charset.StandardCharsets;
import java.util.UUID;

final class OralRecording {
  static final int MAX_BYTES = 20 * 1024 * 1024;
  private MediaRecorder recorder;
  int slot = -2;

  static File clip(File folder, String session, int slot) {
    if (slot < -1 || slot > 2) throw new IllegalArgumentException("Invalid recording target");
    return new File(folder, "oral-" + UUID.fromString(session) + "-" + slot + ".m4a");
  }

  static void validate(File file) throws IOException {
    if (!file.isFile() || file.length() <= 12 || file.length() > MAX_BYTES)
      throw new IOException("Invalid audio length");
    try (RandomAccessFile data = new RandomAccessFile(file, "r")) {
      data.seek(4);
      byte[] marker = new byte[4];
      data.readFully(marker);
      if (!"ftyp".equals(new String(marker, StandardCharsets.US_ASCII)))
        throw new IOException("Invalid audio container");
    }
  }

  @SuppressWarnings("deprecation")
  void start(Context ctx, File file, int target, int seconds, Runnable finished)
      throws IOException {
    if (recorder != null || file.exists()) throw new IOException("Recording already exists");
    MediaRecorder next = Build.VERSION.SDK_INT >= 31 ? new MediaRecorder(ctx) : new MediaRecorder();
    try {
      next.setAudioSource(MediaRecorder.AudioSource.MIC);
      next.setOutputFormat(MediaRecorder.OutputFormat.MPEG_4);
      next.setAudioEncoder(MediaRecorder.AudioEncoder.AAC);
      next.setAudioChannels(1);
      next.setAudioSamplingRate(24000);
      next.setAudioEncodingBitRate(48000);
      next.setMaxFileSize(MAX_BYTES - 65536L);
      next.setMaxDuration(Math.max(1, seconds) * 1000);
      next.setOutputFile(file.getPath());
      next.setOnInfoListener(
          (r, what, extra) -> {
            if (recorder == r
                && (what == MediaRecorder.MEDIA_RECORDER_INFO_MAX_DURATION_REACHED
                    || what == MediaRecorder.MEDIA_RECORDER_INFO_MAX_FILESIZE_REACHED))
              finished.run();
          });
      next.setOnErrorListener(
          (r, what, extra) -> {
            if (recorder == r) finished.run();
          });
      next.prepare();
      next.start();
      recorder = next;
      slot = target;
    } catch (RuntimeException | IOException e) {
      next.release();
      if (file.exists()) file.delete();
      throw new IOException("Microphone unavailable", e);
    }
  }

  boolean stop() {
    if (recorder == null) return false;
    boolean ok = true;
    try {
      recorder.stop();
    } catch (RuntimeException e) {
      ok = false;
    } finally {
      recorder.release();
      recorder = null;
      slot = -2;
    }
    return ok;
  }
}
