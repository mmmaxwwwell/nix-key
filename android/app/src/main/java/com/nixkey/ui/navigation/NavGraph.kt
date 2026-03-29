package com.nixkey.ui.navigation

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController

object Routes {
    const val TAILSCALE_AUTH = "tailscale_auth"
    const val SERVER_LIST = "server_list"
    const val PAIRING = "pairing"
    const val KEY_MANAGEMENT = "key_management/{hostId}"
    const val KEY_DETAIL = "key_detail/{keyId}"
    const val KEY_DETAIL_NEW = "key_detail/new"
    const val SETTINGS = "settings"

    fun keyManagement(hostId: String) = "key_management/$hostId"
    fun keyDetail(keyId: String) = "key_detail/$keyId"
}

@Composable
fun NixKeyNavGraph() {
    val navController = rememberNavController()

    NavHost(
        navController = navController,
        startDestination = Routes.SERVER_LIST,
    ) {
        composable(Routes.TAILSCALE_AUTH) {
            PlaceholderScreen("Tailscale Auth")
        }
        composable(Routes.SERVER_LIST) {
            PlaceholderScreen("Server List")
        }
        composable(Routes.PAIRING) {
            PlaceholderScreen("Pairing")
        }
        composable(Routes.KEY_MANAGEMENT) {
            PlaceholderScreen("Key Management")
        }
        composable(Routes.KEY_DETAIL) {
            PlaceholderScreen("Key Detail")
        }
        composable(Routes.KEY_DETAIL_NEW) {
            PlaceholderScreen("Create Key")
        }
        composable(Routes.SETTINGS) {
            PlaceholderScreen("Settings")
        }
    }
}

@Composable
private fun PlaceholderScreen(name: String) {
    Box(
        modifier = Modifier.fillMaxSize(),
        contentAlignment = Alignment.Center,
    ) {
        Text(text = name)
    }
}
