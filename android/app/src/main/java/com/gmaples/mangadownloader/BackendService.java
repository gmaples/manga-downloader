package com.gmaples.mangadownloader;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.os.IBinder;

/**
 * Keeps the backend running while the app is in the background, so downloads
 * go on, with the notification Android requires for that. Its Quit action
 * stops the backend and the app.
 */
public class BackendService extends Service {
    static final String ACTION_QUIT = "com.gmaples.mangadownloader.QUIT";
    private static final String CHANNEL = "downloads";
    private static final int NOTIFICATION = 1;

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && ACTION_QUIT.equals(intent.getAction())) {
            Backend.stop();
            stopForeground(true);
            stopSelf();
            MainActivity.finishIfShown();
            return START_NOT_STICKY;
        }
        Notification n = notification();
        if (Build.VERSION.SDK_INT >= 29) {
            startForeground(NOTIFICATION, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
        } else {
            startForeground(NOTIFICATION, n);
        }
        Backend.start(this);
        return START_STICKY;
    }

    @Override
    public void onDestroy() {
        Backend.stop();
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    private Notification notification() {
        Notification.Builder b;
        if (Build.VERSION.SDK_INT >= 26) {
            NotificationManager nm = getSystemService(NotificationManager.class);
            nm.createNotificationChannel(new NotificationChannel(CHANNEL,
                    getString(R.string.notification_channel), NotificationManager.IMPORTANCE_LOW));
            b = new Notification.Builder(this, CHANNEL);
        } else {
            b = new Notification.Builder(this);
        }
        int immutable = Build.VERSION.SDK_INT >= 23 ? PendingIntent.FLAG_IMMUTABLE : 0;
        PendingIntent open = PendingIntent.getActivity(this, 0,
                new Intent(this, MainActivity.class), immutable);
        PendingIntent quit = PendingIntent.getService(this, 1,
                new Intent(this, BackendService.class).setAction(ACTION_QUIT), immutable);
        return b.setSmallIcon(android.R.drawable.stat_sys_download)
                .setContentTitle(getString(R.string.app_name))
                .setContentText(getString(R.string.notification_text))
                .setContentIntent(open)
                .setOngoing(true)
                .addAction(new Notification.Action.Builder(null, getString(R.string.notification_stop), quit).build())
                .build();
    }
}
