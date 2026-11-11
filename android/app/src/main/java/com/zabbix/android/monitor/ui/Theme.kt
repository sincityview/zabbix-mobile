package com.zabbix.android.monitor.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable

/**
 * Тема приложения.
 *
 * В Go-версии тема была отдельной настройкой (по умолчанию тёмная), потому что
 * fyne не умел следовать системной. На Android достаточно следовать системе;
 * ручной переключатель добавим вместе с экраном настроек, если понадобится.
 */
@Composable
fun MonitorTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    MaterialTheme(
        colorScheme = if (darkTheme) darkColorScheme() else lightColorScheme(),
        content = content,
    )
}
