package fr.prepamathscrpe.mobile;

import static org.junit.Assert.*;

import java.io.*;
import java.nio.file.Files;
import java.util.UUID;
import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;

public class OralRecordingTest {
  @Rule public TemporaryFolder tmp = new TemporaryFolder();

  @Test
  public void scopedTargetsAndAudioValidation() throws Exception {
    String id = UUID.randomUUID().toString();
    File a = tmp.newFolder("owner-A"), b = tmp.newFolder("owner-B");
    assertNotEquals(OralRecording.clip(a, id, -1), OralRecording.clip(b, id, -1));
    assertNotEquals(OralRecording.clip(a, id, 0), OralRecording.clip(a, id, 1));
    assertThrows(IllegalArgumentException.class, () -> OralRecording.clip(a, "../other", 0));
    assertThrows(IllegalArgumentException.class, () -> OralRecording.clip(a, id, 3));
    File recording = OralRecording.clip(a, id, -1);
    assertThrows(IOException.class, () -> OralRecording.validate(recording));
    Files.write(recording.toPath(), new byte[16]);
    assertThrows(IOException.class, () -> OralRecording.validate(recording));
    try (RandomAccessFile f = new RandomAccessFile(recording, "rw")) {
      f.seek(4);
      f.writeBytes("ftyp");
      OralRecording.validate(recording);
      f.setLength(OralRecording.MAX_BYTES + 1L);
      assertThrows(IOException.class, () -> OralRecording.validate(recording));
    }
  }
}
