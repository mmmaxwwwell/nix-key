package com.nixkey.ui.navigation

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.nixkey.ui.screens.KeyDetailScreen
import com.nixkey.ui.screens.KeyListScreen
import com.nixkey.ui.screens.PairingScreen
import com.nixkey.ui.screens.ServerListScreen
import com.nixkey.ui.screens.SettingsScreen

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
            ServerListScreen(
                onNavigateToSettings = { navController.navigate(Routes.SETTINGS) },
                onNavigateToScanQr = { navController.navigate(Routes.PAIRING) },
                onNavigateToKeys = { hostId -> navController.navigate(Routes.keyManagement(hostId)) },
            )
        }
        composable(Routes.PAIRING) {
            PairingScreen(
                onBack = { navController.popBackStack() },
                onPairingComplete = {
                    navController.popBackStack(Routes.SERVER_LIST, inclusive = false)
                },
            )
        }
        composable(
            route = Routes.KEY_MANAGEMENT,
            arguments = listOf(navArgument("hostId") { type = NavType.StringType }),
        ) { backStackEntry ->
            val hostId = backStackEntry.arguments?.getString("hostId") ?: ""
            KeyListScreen(
                hostId = hostId,
                onBack = { navController.popBackStack() },
                onNavigateToKeyDetail = { keyAlias ->
                    navController.navigate(Routes.keyDetail(keyAlias))
                },
                onNavigateToCreateKey = { navController.navigate(Routes.KEY_DETAIL_NEW) },
            )
        }
        composable(
            route = Routes.KEY_DETAIL,
            arguments = listOf(navArgument("keyId") { type = NavType.StringType }),
        ) {
            KeyDetailScreen(
                onBack = { navController.popBackStack() },
            )
        }
        composable(Routes.KEY_DETAIL_NEW) {
            KeyDetailScreen(
                onBack = { navController.popBackStack() },
            )
        }
        composable(Routes.SETTINGS) {
            SettingsScreen(
                onBack = { navController.popBackStack() },
            )
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
