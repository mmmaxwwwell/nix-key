package com.nixkey.tailscale

/**
 * Abstraction over the libtailscale userspace Tailscale implementation.
 *
 * This interface exists so that [TailscaleManager] can be tested with a mock
 * backend. The production implementation wraps the gomobile-generated
 * libtailscale bindings.
 */
interface TailscaleBackend {
    /**
     * Start the Tailscale node. If [authKey] is non-null, use it as a
     * pre-authorized key to join the Tailnet automatically. Otherwise,
     * return an OAuth URL for interactive authentication.
     *
     * @param authKey Pre-authorized Tailscale auth key, or null for OAuth flow
     * @param dataDir Directory for Tailscale state files
     * @return An OAuth URL if interactive auth is needed, or null if
     *         auth completed via [authKey]
     */
    fun start(authKey: String?, dataDir: String): String?

    /** Stop the Tailscale node and release resources. */
    fun stop()

    /**
     * Get the Tailscale IP address assigned to this node.
     * @return The Tailscale IPv4 address (e.g., "100.x.x.x"), or null if not connected.
     */
    fun getIp(): String?

    /** Returns true if the Tailscale node is currently running and connected. */
    fun isRunning(): Boolean
}
