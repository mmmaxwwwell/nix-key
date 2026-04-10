# nix-key ProGuard rules

# BouncyCastle - keep all crypto providers
-keep class org.bouncycastle.** { *; }
-dontwarn org.bouncycastle.**

# Timber - remove debug logging in release
-assumenosideeffects class timber.log.Timber {
    public static void d(...);
    public static void v(...);
}
