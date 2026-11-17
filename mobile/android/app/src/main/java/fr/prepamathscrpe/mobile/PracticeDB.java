package fr.prepamathscrpe.mobile;

import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;
import android.database.sqlite.SQLiteOpenHelper;
import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

final class PracticeDB extends SQLiteOpenHelper {
  PracticeDB(Context context) {
    super(context, "practice.db", null, 1);
  }

  @Override
  public void onCreate(SQLiteDatabase db) {
    db.execSQL(
        "CREATE TABLE records(owner TEXT NOT NULL,kind TEXT NOT NULL,id TEXT NOT NULL,payload TEXT"
            + " NOT NULL,image BLOB,PRIMARY KEY(owner,kind,id))");
  }

  @Override
  public void onConfigure(SQLiteDatabase db) {
    super.onConfigure(db);
    try (Cursor pragma = db.rawQuery("PRAGMA secure_delete=ON", null)) {
      pragma.moveToFirst();
    }
  }

  @Override
  public void onUpgrade(SQLiteDatabase db, int oldVersion, int newVersion) {
    throw new IllegalStateException("Unsupported practice database version");
  }

  void put(String owner, String kind, String id, JSONObject json, byte[] image) {
    ContentValues values = new ContentValues();
    values.put("payload", json.toString());
    if (image != null) values.put("image", image);
    SQLiteDatabase db = getWritableDatabase();
    db.beginTransaction();
    try {
      if (db.update(
              "records", values, "owner=? AND kind=? AND id=?", new String[] {owner, kind, id})
          == 0) {
        values.put("owner", owner);
        values.put("kind", kind);
        values.put("id", id);
        db.insertOrThrow("records", null, values);
      }
      db.setTransactionSuccessful();
    } finally {
      db.endTransaction();
    }
  }

  JSONArray rows(String owner, String kind) throws JSONException {
    JSONArray rows = new JSONArray();
    try (Cursor cursor =
        getReadableDatabase()
            .rawQuery(
                "SELECT payload FROM records WHERE owner=? AND kind=? ORDER BY rowid",
                new String[] {owner, kind})) {
      while (cursor.moveToNext()) rows.put(new JSONObject(cursor.getString(0)));
    }
    return rows;
  }

  byte[] picture(String owner, String id) {
    // CursorWindow is smaller than a permitted JPEG. Read bounded chunks from SQLite.
    try (Cursor sizes =
        getReadableDatabase()
            .rawQuery(
                "SELECT length(image) FROM records WHERE owner=? AND kind='copy' AND id=?",
                new String[] {owner, id})) {
      if (!sizes.moveToFirst() || sizes.isNull(0)) return null;
      int length = sizes.getInt(0);
      if (length < 1 || length > 8 * 1024 * 1024)
        throw new IllegalStateException("Invalid image size");
      byte[] bytes = new byte[length];
      for (int pos = 0; pos < length; pos += 512 * 1024) {
        try (Cursor chunk =
            getReadableDatabase()
                .rawQuery(
                    "SELECT substr(image,?,524288) FROM records WHERE owner=? AND kind='copy' AND"
                        + " id=?",
                    new String[] {String.valueOf(pos + 1), owner, id})) {
          if (!chunk.moveToFirst()) throw new IllegalStateException("Missing image");
          byte[] slice = chunk.getBlob(0);
          System.arraycopy(slice, 0, bytes, pos, slice.length);
        }
      }
      return bytes;
    }
  }

  void remove(String owner, String kind, String id) {
    if (kind == null) getWritableDatabase().delete("records", "owner=?", new String[] {owner});
    else if (id == null)
      getWritableDatabase().delete("records", "owner=? AND kind=?", new String[] {owner, kind});
    else
      getWritableDatabase()
          .delete("records", "owner=? AND kind=? AND id=?", new String[] {owner, kind, id});
  }
}
