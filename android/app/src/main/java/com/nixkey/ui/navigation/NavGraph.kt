package com.nixkey.ui.navigation

import android.net.Uri
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
import com.nixkey.ui.screens.TailscaleAuthScreen

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
    fun pairingWithPayload(payload: String) = "pairing?payload=${Uri.encode(payload)}"
}

@Composable
fun NixKeyNavGraph(
    needsTailscaleAuth: Boolean = false,
    deepLinkPayload: String? = null,
    onDeepLinkConsumed: () -> Unit = {}
) {
    val navController = rememberNavController()

    val startDestination = if (needsTailscaleAuth) Routes.TAILSCALE_AUTH else Routes.SERVER_LIST

    LaunchedEffect(deepLinkPayload) {
        if (deepLinkPayload != null) {
            navController.navigate(Routes.pairingWithPayload(deepLinkPayload))
            onDeepLinkConsumed()
        }
    }

    NavHost(
        navController = navController,
        startDestination = startDestination
    ) {
        composable(Routes.TAILSCALE_AUTH) {
            TailscaleAuthScreen(
                onAuthSuccess = {
                    navController.navigate(Routes.SERVER_LIST) {
                        popUpTo(Routes.TAILSCALE_AUTH) { inclusive = true }
                    }
                }
            )
        }
        composable(Routes.SERVER_LIST) {
            ServerListScreen(
                onNavigateToSettings = { navController.navigate(Routes.SETTINGS) },
                onNavigateToScanQr = { navController.navigate(Routes.PAIRING) },
                onNavigateToKeys = { hostId -> navController.navigate(Routes.keyManagement(hostId)) }
            )
        }
        composable(
            route = "${Routes.PAIRING}?payload={payload}",
            arguments = listOf(
                navArgument("payload") {
                    type = NavType.StringType
                    nullable = true
                    defaultValue = null
                }
            )
        ) { backStackEntry ->
            val initialPayload = backStackEntry.arguments?.getString("payload")
            PairingScreen(
                onBack = { navController.popBackStack() },
                onPairingComplete = {
                    navController.popBackStack(Routes.SERVER_LIST, inclusive = false)
                },
                initialPayload = initialPayload
            )
        }
        composable(
            route = Routes.KEY_MANAGEMENT,
            arguments = listOf(navArgument("hostId") { type = NavType.StringType })
        ) { backStackEntry ->
            val hostId = backStackEntry.arguments?.getString("hostId") ?: ""
            KeyListScreen(
                hostId = hostId,
                onBack = { navController.popBackStack() },
                onNavigateToKeyDetail = { keyAlias ->
                    navController.navigate(Routes.keyDetail(keyAlias))
                },
                onNavigateToCreateKey = { navController.navigate(Routes.KEY_DETAIL_NEW) }
            )
        }
        composable(
            route = Routes.KEY_DETAIL,
            arguments = listOf(navArgument("keyId") { type = NavType.StringType })
        ) {
            KeyDetailScreen(
                onBack = { navController.popBackStack() }
            )
        }
        composable(Routes.KEY_DETAIL_NEW) {
            KeyDetailScreen(
                onBack = { navController.popBackStack() }
            )
        }
        composable(Routes.SETTINGS) {
            SettingsScreen(
                onBack = { navController.popBackStack() },
                onReauthenticate = {
                    navController.navigate(Routes.TAILSCALE_AUTH) {
                        popUpTo(Routes.SERVER_LIST) { inclusive = true }
                    }
                }
            )
        }
    }
}
