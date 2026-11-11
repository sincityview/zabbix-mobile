package com.zabbix.android.monitor.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import com.zabbix.android.monitor.data.MonitorSettings
import com.zabbix.android.monitor.data.SettingsStore

/**
 * Два экрана без навигационной библиотеки: пока настроек нет — настройки,
 * иначе список проблем. Настройки открываются кнопкой в шапке списка.
 */
@Composable
fun MonitorApp() {
    val context = LocalContext.current
    val store = remember(context) { SettingsStore(context) }

    var settings by remember { mutableStateOf(store.load()) }
    var editing by remember { mutableStateOf(!settings.isConfigured) }
    // Сортировка живёт отдельно от MonitorSettings, чтобы её изменение не
    // пересоздавало экран списка (иначе каждый тап по меню — новый запрос).
    var sort by remember { mutableStateOf(store.loadSort()) }

    if (editing) {
        SettingsScreen(
            initial = settings,
            canCancel = settings.isConfigured,
            onCancel = { editing = false },
            onSaved = { saved ->
                // normalized() использует copy, поэтому лимит, интервал и любые
                // будущие поля настроек не теряются при сохранении
                val normalized = saved.normalized()
                store.save(normalized)
                settings = normalized
                editing = false
            },
        )
    } else {
        ProblemListScreen(
            settings = settings,
            sort = sort,
            onSortChange = { chosen ->
                sort = chosen
                store.saveSort(chosen)
            },
            onOpenSettings = { editing = true },
        )
    }
}
