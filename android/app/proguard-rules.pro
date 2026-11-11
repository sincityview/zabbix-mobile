# R8: правила для Compose и AndroidX подключаются из AAR-метаданных,
# поэтому здесь пока только то, что нужно самому приложению.
-keepattributes *Annotation*

# Отладочная информация в стектрейсах release-сборки: без неё падения
# нечитаемы, а весит немного после сжатия.
-keepattributes SourceFile,LineNumberTable
-renamesourcefileattribute SourceFile
