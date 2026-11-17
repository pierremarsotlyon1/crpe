package fr.prepamathscrpe.mobile;

import android.content.*;
import android.database.*;
import android.net.Uri;
import android.os.*;
import android.provider.OpenableColumns;
import java.io.*;
import java.util.List;

public final class LocalFiles extends ContentProvider {
  static File folder(Context c, String owner) throws IOException {
    if (owner == null || !owner.matches("[A-Fa-f0-9-]{36}")) throw new IOException("Invalid owner");
    File folder = new File(c.getCacheDir(), "member-" + owner);
    if (!folder.isDirectory() && !folder.mkdir()) throw new IOException("Cannot create cache");
    return folder;
  }

  static Uri uri(Context c, String owner, String name) {
    return new Uri.Builder()
        .scheme("content")
        .authority(c.getPackageName() + ".files")
        .appendPath(owner)
        .appendPath(name)
        .build();
  }

  private File resolve(Uri uri) throws FileNotFoundException {
    List<String> parts = uri.getPathSegments();
    if (parts.size() != 2
        || !parts.get(0).matches("[A-Fa-f0-9-]{36}")
        || !parts.get(1).matches("([A-Fa-f0-9-]{36}\\.(pdf|jpg)|mes-donnees-crpe\\.json)"))
      throw new FileNotFoundException();
    try {
      return new File(folder(getContext(), parts.get(0)), parts.get(1));
    } catch (IOException e) {
      throw new FileNotFoundException();
    }
  }

  @Override
  public boolean onCreate() {
    return true;
  }

  @Override
  public String getType(Uri uri) {
    String last = uri.getLastPathSegment();
    return last != null && last.endsWith(".pdf")
        ? "application/pdf"
        : last != null && last.endsWith(".jpg") ? "image/jpeg" : "application/json";
  }

  @Override
  public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
    File file = resolve(uri);
    if (mode.contains("w") && !file.getName().endsWith(".jpg")) throw new FileNotFoundException();
    return ParcelFileDescriptor.open(file, ParcelFileDescriptor.parseMode(mode));
  }

  @Override
  public Cursor query(Uri uri, String[] projection, String selection, String[] args, String order) {
    try {
      File f = resolve(uri);
      String[] keys =
          projection == null
              ? new String[] {OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE}
              : projection;
      MatrixCursor c = new MatrixCursor(keys);
      Object[] row = new Object[keys.length];
      for (int i = 0; i < keys.length; i++) {
        if (OpenableColumns.DISPLAY_NAME.equals(keys[i])) row[i] = f.getName();
        if (OpenableColumns.SIZE.equals(keys[i])) row[i] = f.length();
      }
      c.addRow(row);
      return c;
    } catch (IOException e) {
      return null;
    }
  }

  @Override
  public Uri insert(Uri uri, ContentValues values) {
    throw new UnsupportedOperationException();
  }

  @Override
  public int delete(Uri uri, String selection, String[] args) {
    throw new UnsupportedOperationException();
  }

  @Override
  public int update(Uri uri, ContentValues values, String s, String[] args) {
    throw new UnsupportedOperationException();
  }
}
