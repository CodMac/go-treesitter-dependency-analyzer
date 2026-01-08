package com.example.model;

/**
 * NotificationException - 自定义异常，用于测试 EXTEND 和 THROW 关系。
 */
public class NotificationException extends Exception {

    // Contain: NotificationException CONTAIN Field (serialVersionUID)
    private static final long serialVersionUID = 1L;

    /**
     * Extend: NotificationException EXTEND Exception
     * Use: Exception Type in constructor parameter.
     */
    public NotificationException(String message, Throwable cause) {
        super(message, cause);
    }

    /**
     * Use: ErrorCode Type in constructor parameter.
     */
    public NotificationException(ErrorCode code) {
        // Use: code.getMessage()
        super(code.getMessage());
    }
}