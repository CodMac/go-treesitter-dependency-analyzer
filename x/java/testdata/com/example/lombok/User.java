package com.example.lombok;

import lombok.Getter;
import lombok.Setter;
import lombok.Data;

@Data
public class User {
    private String name;
    private int age;
}

public class UserService {
    public void process() {
        User user = new User();
        // 重点：源码中没有 setName 和 getName，分析引擎需识别它们
        user.setName("Alice");
        System.out.println(user.getName());
    }
}