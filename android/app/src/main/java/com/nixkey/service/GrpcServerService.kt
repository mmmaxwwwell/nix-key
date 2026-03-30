package com.nixkey.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.IBinder
import androidx.core.app.NotificationCompat
import com.nixkey.MainActivity
import com.nixkey.bridge.GoPhoneServer
import com.nixkey.data.SettingsRepository
import com.nixkey.tailscale.TailscaleManager
import dagger.hilt.android.AndroidEntryPoint
import timber.log.Timber
import java.net.BindException
import javax.annotation.concurrent.ThreadSafe
import javax.inject.Inject

/**
 * Android foreground service that manages the gRPC server lifecycle.
 *
 * When started, this service:
 * 1. Starts Tailscale via [TailscaleManager]
 * 2. Binds the gRPC server to the Tailscale IP + configured port with mTLS
 * 3. Shows a persistent "nix-key active" notification
 *
 * When stopped, the service tears down the gRPC server and Tailscale node.
 *
 * [FR-012]: gRPC server bound to Tailscale interface.
 * [FR-014]: The phone's TLS server MUST bind only to the Tailscale interface.
 */
@ThreadSafe
@AndroidEntryPoint
class GrpcServerService : Service() {

    @Inject lateinit var tailscaleManager: TailscaleManager
    @Inject lateinit var goPhoneServer: GoPhoneServer
    @Inject lateinit var settingsRepository: SettingsRepository

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> {
                stopServer()
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
                return START_NOT_STICKY
            }
        }

        startForeground(NOTIFICATION_ID, buildNotification("Starting nix-key..."))
        startServer()
        return START_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        stopServer()
        super.onDestroy()
    }

    private fun startServer() {
        try {
            // Start Tailscale if not already running
            if (!tailscaleManager.isRunning()) {
                val oauthUrl = tailscaleManager.start()
                if (oauthUrl != null) {
                    Timber.w("GrpcServerService: Tailscale requires OAuth, cannot start server")
                    updateNotification("Tailscale auth required")
                    return
                }
            }

            val ip = tailscaleManager.getIp()
            if (ip == null) {
                // FR-115: detect stale auth and trigger re-auth instead of staying stuck
                if (tailscaleManager.handleStaleAuth()) {
                    Timber.w("GrpcServerService: stale Tailscale auth, re-authentication required")
                    updateNotification("Tailscale re-auth required")
                } else {
                    Timber.e("GrpcServerService: no Tailscale IP available")
                    updateNotification("No Tailscale IP")
                }
                return
            }

            val port = settingsRepository.listenPort
            val address = "$ip:$port"

            // Pass OTEL endpoint if tracing is enabled (FR-088)
            val otelEndpoint = if (settingsRepository.otelEnabled) {
                settingsRepository.otelEndpoint.ifEmpty { null }
            } else {
                null
            }

            // FR-014: bind only to Tailscale interface
            goPhoneServer.start(address, otelEndpoint)
            Timber.i("GrpcServerService: gRPC server started on %s", address)
            updateNotification("nix-key active")
        } catch (e: BindException) {
            Timber.e(e, "GrpcServerService: port %d already in use", settingsRepository.listenPort)
            updateNotification("Port ${settingsRepository.listenPort} already in use")
        } catch (e: Exception) {
            val isPortConflict = e.cause is BindException ||
                e.message?.contains("address already in use", ignoreCase = true) == true ||
                e.message?.contains("EADDRINUSE", ignoreCase = true) == true
            if (isPortConflict) {
                Timber.e(e, "GrpcServerService: port conflict on port %d", settingsRepository.listenPort)
                updateNotification("Port ${settingsRepository.listenPort} already in use")
            } else {
                Timber.e(e, "GrpcServerService: failed to start server")
                updateNotification("Server error: ${e.message}")
            }
        }
    }

    private fun stopServer() {
        try {
            if (goPhoneServer.isRunning()) {
                goPhoneServer.stop()
                Timber.i("GrpcServerService: gRPC server stopped")
            }
        } catch (e: Exception) {
            Timber.e(e, "GrpcServerService: error stopping gRPC server")
        }

        try {
            if (tailscaleManager.isRunning()) {
                tailscaleManager.stop()
                Timber.i("GrpcServerService: Tailscale stopped")
            }
        } catch (e: Exception) {
            Timber.e(e, "GrpcServerService: error stopping Tailscale")
        }
    }

    private fun createNotificationChannel() {
        val channel = NotificationChannel(
            CHANNEL_ID,
            "nix-key Server",
            NotificationManager.IMPORTANCE_LOW,
        ).apply {
            description = "Shows when the nix-key gRPC server is active"
            setShowBadge(false)
        }
        val nm = getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(channel)
    }

    private fun buildNotification(text: String): Notification {
        val openIntent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP
        }
        val pendingIntent = PendingIntent.getActivity(
            this,
            0,
            openIntent,
            PendingIntent.FLAG_IMMUTABLE,
        )

        val stopIntent = Intent(this, GrpcServerService::class.java).apply {
            action = ACTION_STOP
        }
        val stopPendingIntent = PendingIntent.getService(
            this,
            1,
            stopIntent,
            PendingIntent.FLAG_IMMUTABLE,
        )

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("nix-key")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.ic_lock_lock)
            .setOngoing(true)
            .setContentIntent(pendingIntent)
            .addAction(
                android.R.drawable.ic_menu_close_clear_cancel,
                "Stop",
                stopPendingIntent,
            )
            .build()
    }

    private fun updateNotification(text: String) {
        val nm = getSystemService(NotificationManager::class.java)
        nm.notify(NOTIFICATION_ID, buildNotification(text))
    }

    companion object {
        private const val CHANNEL_ID = "nixkey_server"
        private const val NOTIFICATION_ID = 1
        private const val ACTION_STOP = "com.nixkey.action.STOP_SERVER"

        fun startService(context: Context) {
            val intent = Intent(context, GrpcServerService::class.java)
            context.startForegroundService(intent)
        }

        fun stopService(context: Context) {
            val intent = Intent(context, GrpcServerService::class.java).apply {
                action = ACTION_STOP
            }
            context.startService(intent)
        }
    }
}
