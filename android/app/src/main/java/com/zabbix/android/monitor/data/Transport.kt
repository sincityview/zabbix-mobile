package com.zabbix.android.monitor.data

import java.io.IOException
import java.net.HttpURLConnection
import java.net.URI
import java.security.SecureRandom
import java.security.cert.X509Certificate
import javax.net.ssl.HostnameVerifier
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager

/**
 * HTTP-транспорт вынесен за интерфейс, чтобы сетевой слой проверялся
 * обычными JVM-тестами (без Android и без живого сервера).
 */
fun interface HttpTransport {
    fun post(url: String, token: String, body: String, allowInsecureTls: Boolean): String
}

/** Транспорт на HttpURLConnection: без сторонних зависимостей. */
object UrlConnectionTransport : HttpTransport {

    private const val TIMEOUT_MS = 20_000

    override fun post(url: String, token: String, body: String, allowInsecureTls: Boolean): String {
        val connection = URI(url).toURL().openConnection() as HttpURLConnection
        try {
            connection.requestMethod = "POST"
            connection.connectTimeout = TIMEOUT_MS
            connection.readTimeout = TIMEOUT_MS
            connection.doOutput = true
            connection.setRequestProperty("Content-Type", "application/json")
            connection.setRequestProperty("Accept", "application/json")
            connection.setRequestProperty("User-Agent", "zabbix-monitor-android")
            if (token.isNotEmpty()) {
                connection.setRequestProperty("Authorization", "Bearer $token")
            }
            if (allowInsecureTls && connection is HttpsURLConnection) {
                connection.sslSocketFactory = insecureSslContext().socketFactory
                connection.hostnameVerifier = HostnameVerifier { _, _ -> true }
            }

            connection.outputStream.use { it.write(body.toByteArray(Charsets.UTF_8)) }

            val code = connection.responseCode
            val stream = if (code in 200..299) connection.inputStream else connection.errorStream
            val text = stream?.bufferedReader()?.use { it.readText() }.orEmpty()

            if (code !in 200..299) {
                throw IOException("HTTP $code: ${text.take(200)}")
            }
            return text
        } finally {
            connection.disconnect()
        }
    }

    /**
     * Доверие самоподписанному сертификату — только если пользователь включил
     * этот флаг в настройках: сеть и так под его контролем, но по умолчанию
     * TLS проверяется нормально.
     */
    @Suppress("TrustAllX509TrustManager", "CustomX509TrustManager")
    private fun insecureSslContext(): SSLContext {
        val trustAll = arrayOf<TrustManager>(
            object : X509TrustManager {
                override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) = Unit
                override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) = Unit
                override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
            },
        )
        return SSLContext.getInstance("TLS").apply { init(null, trustAll, SecureRandom()) }
    }
}
