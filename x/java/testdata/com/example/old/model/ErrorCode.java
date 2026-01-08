package com.example.model;

/**
 * ErrorCode - 用于测试 ENUM 定义和使用。
 */
public enum ErrorCode {
    // Contain: ErrorCode CONTAIN Enum Constant (USER_NOT_FOUND, NAME_EMPTY)
    USER_NOT_FOUND(404, "User not found in repository"),
    NAME_EMPTY(400, "User name cannot be empty");

    // Contain: ErrorCode CONTAIN Field (code, message)
    private final int code;
    private final String message;

    // Contain: ErrorCode CONTAIN Constructor (ErrorCode)
    ErrorCode(int code, String message) {
        this.code = code;
        this.message = message;
    }

    // Contain: ErrorCode CONTAIN Method (getCode)
    public int getCode() {
        return code;
    }

    // Contain: ErrorCode CONTAIN Method (getMessage)
    public String getMessage() {
        return message;
    }
}