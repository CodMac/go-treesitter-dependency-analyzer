package com.example.config;

public class ConfigService {
    // 边界：变长参数 ... 和 多维数组 []
    public void updateConfigs(String[] keys, Object... values) {
    }

    // 边界：复杂注解
    @SuppressWarnings({"unchecked", "rawtypes"})
    @Deprecated(since = "1.2", forRemoval = true)
    public void legacyMethod() {
    }
}