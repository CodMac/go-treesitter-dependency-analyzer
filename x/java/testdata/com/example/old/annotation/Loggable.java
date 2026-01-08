package com.example.annotation;

import java.lang.annotation.*;

/**
 * 测试：Annotation Type Declaration & Meta-Annotations
 * 附加点：验证语义化 Import （"*"通配符）
 * 附加点：验证注释提取
 */
@Retention(RetentionPolicy.RUNTIME)
@Target({ElementType.TYPE, ElementType.METHOD})
public @interface Loggable {
    String level() default "INFO";

    boolean trace() default false;
}