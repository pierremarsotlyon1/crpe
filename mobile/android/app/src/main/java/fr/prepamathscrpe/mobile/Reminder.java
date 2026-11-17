package fr.prepamathscrpe.mobile;

import static android.content.Context.*;

import android.app.*;
import android.content.*;
import java.time.*;

public final class Reminder extends BroadcastReceiver {
  private static final String FIRE = "fr.prepamathscrpe.mobile.DAILY";
  private static final String CHANNEL = "revision";

  static int hour(Context c) {
    return c.getSharedPreferences("reminders", MODE_PRIVATE).getInt("hour", -1);
  }

  static void change(Context c, int hour) {
    c.getSharedPreferences("reminders", MODE_PRIVATE).edit().putInt("hour", hour).apply();
    AlarmManager alarm = c.getSystemService(AlarmManager.class);
    PendingIntent pending =
        PendingIntent.getBroadcast(
            c,
            19,
            new Intent(c, Reminder.class).setAction(FIRE),
            PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    alarm.cancel(pending);
    if (hour < 0) return;
    ZonedDateTime now = ZonedDateTime.now(),
        next = now.toLocalDate().atTime(hour, 0).atZone(now.getZone());
    if (!next.isAfter(now)) next = next.plusDays(1);
    alarm.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, next.toInstant().toEpochMilli(), pending);
  }

  @Override
  public void onReceive(Context c, Intent intent) {
    int hour = hour(c);
    if (hour < 0) return;
    NotificationManager manager = c.getSystemService(NotificationManager.class);
    manager.createNotificationChannel(
        new NotificationChannel(
            CHANNEL, "Révision quotidienne", NotificationManager.IMPORTANCE_DEFAULT));
    if (FIRE.equals(intent.getAction()) && manager.areNotificationsEnabled()) {
      PendingIntent launch =
          PendingIntent.getActivity(
              c,
              0,
              new Intent(c, MainActivity.class),
              PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
      Notification n =
          new Notification.Builder(c, CHANNEL)
              .setSmallIcon(R.drawable.icon)
              .setContentTitle("Vos cinq minutes de maths")
              .setContentText("Votre entraînement CRPE vous attend.")
              .setContentIntent(launch)
              .setAutoCancel(true)
              .build();
      manager.notify(19, n);
    }
    change(c, hour);
  }
}
