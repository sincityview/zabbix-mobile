package com.zabbix.android.monitor

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import com.zabbix.android.monitor.ui.MonitorApp
import com.zabbix.android.monitor.ui.MonitorTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            MonitorTheme {
                MonitorApp()
            }
        }
    }
}
