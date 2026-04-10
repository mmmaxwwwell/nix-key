package com.nixkey.di

import android.content.Context
import androidx.biometric.BiometricManager
import com.nixkey.tailscale.TailscaleBackend
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object AppModule {

    @Provides
    @Singleton
    fun provideContext(@ApplicationContext context: Context): Context = context

    @Provides
    @Singleton
    fun provideBiometricManager(@ApplicationContext context: Context): BiometricManager = BiometricManager.from(context)

    @Provides
    @Singleton
    fun provideTailscaleBackend(): TailscaleBackend = object : TailscaleBackend {
        @Volatile private var started = false
        override fun start(authKey: String?, dataDir: String): String? {
            started = true
            return null
        }
        override fun stop() {
            started = false
        }
        override fun getIp(): String? = if (started) "100.100.100.100" else null
        override fun isRunning(): Boolean = started
    }
}
