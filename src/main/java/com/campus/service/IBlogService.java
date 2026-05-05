package com.campus.service;

import com.campus.dto.Result;
import com.campus.entity.Blog;
import com.baomidou.mybatisplus.extension.service.IService;

/**
 * <p>
 *  服务类
 * </p>
 *
 * @author 虎哥
 * @since 2021-12-22
 */
public interface IBlogService extends IService<Blog> {

    Result queryBlogById(Long id);

    Result queryHotBlog(Integer current);

    Result updateLike(Long id);

    Result queryBlogLikes(Long id);

    Result saveBlog(Blog blog);

    Result quertBlogOfFollow(Long max, Integer offset);
}
